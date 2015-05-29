/*
* classifier.go
*
* the classifier thread determines the type of file being handled
*
* Notes:
*    - Each type must have at least 1 test in use.
*    - A type will only be matched if all tests in use pass.
*    - Types will be tested in order of specification in the configuration.
*/

package main

import(
        "errors"
        "fmt"
	"log"
        "os/exec"
	"path/filepath"
        "regexp"
        "strings"

        "github.com/spf13/viper"
)


type TypeInfo struct {
        Name             string
        DoMatchExtension bool
        Extension        string
        DoMatchRegexp    bool
        RegexpTemplate  *regexp.Regexp
        DoHash           bool
        DoNearline       bool
        NearlineCmd      string
}


// ValidateClassifierConfig checks the sanity of the classifier section of a configuration.
// It makes the following guarantees
//   1) Each entry in the "type" array should have a "name"
//   2) Each entry in the "type" array should have at least 1 test in use.
//   3) If "match-regexp" is present, the provided string is a valid regular expression.
func ValidateClassifierConfig() (e error) {
        typesRawIfc := viper.Get("classifier.types")
        if typesRawIfc == nil {
                e = errors.New("[classifier] No types were provided")
                log.Print(e.Error())
                return
        }
        typesRaw := typesRawIfc.([]interface{})
        
        //var types = make([]TypeInfo, len(typesRaw))
        nTestsPresent := uint(0)
        for iType, typeMapIfc := range typesRaw {
                typeMap := typeMapIfc.(map[string](interface{}))
                if typeMap["name"].(string) == "" {
                        e = fmt.Errorf("[classifier] Type %d is missing its name", iType)
                        log.Print(e.Error())
                }
                if _, hasExt := typeMap["match-extension"]; hasExt {
                        nTestsPresent++
                }
                if regexpTemplate, hasRegexp := typeMap["match-regexp"]; hasRegexp {
                        nTestsPresent++
                        if _, regexpErr := regexp.Compile(regexpTemplate.(string)); regexpErr != nil {
                                e = fmt.Errorf("[classifier] Invalid regular expression: %s\n\t%v", regexpTemplate.(string), regexpErr.Error())
                                log.Printf(e.Error())
                        }
                }
                if nTestsPresent == 0 {
                        e = fmt.Errorf("[classifier] No tests are present for type %d", iType)
                        log.Print(e.Error())
                }
        }
	return
}




func Classifier(context OperatorContext) {
	// decrement the wg counter at the end
	defer context.PoolCount.Done()

        typesRawIfc := viper.Get("classifier.types")
        typesRaw := typesRawIfc.([]interface{})
        
        var types = make([]TypeInfo, len(typesRaw))
        for iType, typeMapIfc := range typesRaw {
                typeMap := typeMapIfc.(map[string](interface{}))
                types[iType].Name = typeMap["name"].(string)
                types[iType].DoMatchExtension = false
                if ext, hasExt := typeMap["match-extension"]; hasExt {
                        types[iType].DoMatchExtension = true
                        types[iType].Extension = "." + ext.(string)
                }
                types[iType].DoMatchRegexp = false
                if regexpTemplate, hasRegexp := typeMap["match-regexp"]; hasRegexp {
                        types[iType].DoMatchRegexp = true
                        types[iType].RegexpTemplate = regexp.MustCompile(regexpTemplate.(string))
                }
                types[iType].DoHash = typeMap["do-hash"].(bool)
                types[iType].DoNearline = typeMap["do-nearline"].(bool)
                if types[iType].DoNearline {
                        types[iType].NearlineCmd = typeMap["nearline-cmd"].(string)
                }
                log.Printf("[classifier] Adding type:\n\t%v", types[iType])
        }

        requireHash := viper.GetBool("classifier.require-hash")
        hashCmd := "md5sum"
        hashOpt := "-b"

	log.Print("[classifier] started successfully")

shipLoop:
	for {
		select {
		// the control messages can stop execution
		case controlMsg := <-context.CtrlQueue:
			if controlMsg == StopExecution {
				log.Print("[classifier] stopping on interrupt.")
				break shipLoop
			}
		case fileHeader := <-context.FileStream:
                        inputFilePath := filepath.Join(fileHeader.HotPath, fileHeader.Filename)
                        opReturn := OperatorReturn{
                                     Operator:  "shipper",
                                     FHeader:   fileHeader,
                                     Err:       nil,
                                     IsFatal:     false,
                        }

                        _, inputFilename := filepath.Split(inputFilePath)

                        acceptType := bool(false) 

typeLoop:
                        for _, typeInfo := range types {
                                acceptType = true  // this must start as true for this multi-test setup to work
                                if typeInfo.DoMatchExtension {
                                         acceptType = acceptType && strings.HasSuffix(inputFilename, typeInfo.Extension)
                                }
                                if typeInfo.DoMatchRegexp {
                                         acceptType = acceptType && typeInfo.RegexpTemplate.MatchString(inputFilename)
                                }
                                
                                if acceptType {
                                        log.Printf("[classifier] Classifying file <%s> as type <%s>", inputFilename, typeInfo.Name)
                                        opReturn.FHeader.FileType = typeInfo.Name
                                        opReturn.FHeader.DoNearline = typeInfo.DoNearline
                                        opReturn.FHeader.SetNearlineCmd(typeInfo.NearlineCmd)
                                        if hash, hashErr := exec.Command(hashCmd, hashOpt, inputFilePath).CombinedOutput(); hashErr != nil {
                                                opReturn.Err = hashErr
                                                opReturn.IsFatal = requireHash
                                                log.Printf("[classifier] error while hashing: %s", hashErr.Error())
                                        } else {
                                                hashTokens := strings.Fields(string(hash))
                                                opReturn.FHeader.FileHash = hashTokens[0]
                                                log.Printf("[classifier] file <%s> hash: %s", inputFilename, opReturn.FHeader.FileHash)
                                        }
                                        context.RetStream <- opReturn
                                        break typeLoop
                                }
                                // if we reach this point, we can assume acceptType is false
                                // if this is the last iteration through the loop, acceptType will be false going forward
                        }

                        if acceptType == false {
                                log.Printf("[classifier] Unable to classify file <%s>", inputFilename)
                                opReturn.Err = errors.New("[Classifier] Unable to classify")
                                opReturn.IsFatal = true
                                context.RetStream <- opReturn
                        }
                }

	}

	// Finish any pending move jobs.

	log.Print("[classifier] finished.")
}
