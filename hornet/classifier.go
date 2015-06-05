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

package hornet

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type TypeInfo struct {
	Name             string
	DoMatchExtension bool
	Extension        string
	DoMatchRegexp    bool
	RegexpTemplate   *regexp.Regexp
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

	if viper.IsSet("classifier.base-paths") {
		basePathsRawIfc := viper.Get("classifier.base-paths")
		basePathsRaw := basePathsRawIfc.([]interface{})
		for _, pathIfc := range basePathsRaw {
			if _, fpErr := filepath.Abs(pathIfc.(string)); fpErr != nil {
				e = fmt.Errorf("[classifier] Invalid base path: <%v>", pathIfc.(string))
				log.Printf(e.Error())
			}
		}
	}

	if viper.IsSet("hash") == false {
		e = errors.New("[classifier] Hash configuration not provided")
		log.Print(e.Error())
		return
	}
	return
}

func getSubPath(path string) (subPath string) {
	subPath = ""
	for _, basePath := range BasePaths {
		if strings.HasPrefix(path, basePath) {
			var relErr error
			subPath, relErr = filepath.Rel(basePath, path)
			if relErr != nil {
				log.Printf("[classifier] Unable to get relative path after checking prefix:\n\t%s\n\t%s\n\t%v", basePath, path, relErr)
				subPath = ""
			} else {
				break
			}
		}
	}
	return
}

func Classifier(context OperatorContext) {
	// decrement the wg counter at the end
	defer context.PoolCount.Done()
	defer log.Print("[classifier] finished.")

	// Process the file types
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

	// Process the base paths
	basePathsRawIfc := viper.Get("classifier.base-paths")
	basePathsRaw := make([]interface{}, 0)
	if basePathsRawIfc != nil {
		basePathsRaw = basePathsRawIfc.([]interface{})
	}

	includeWatcherDir := viper.GetBool("watcher.active")

	nBasePaths := len(basePathsRaw)
	if includeWatcherDir {nBasePaths++}

	BasePaths = make([]string, nBasePaths)
	firstPath := 0
	if includeWatcherDir {
		BasePaths[0], _ = filepath.Abs(viper.GetString("watcher.dir"))
		firstPath++
	}
	for iPath, pathIfc := range basePathsRaw {
		BasePaths[firstPath + iPath], _ = filepath.Abs(pathIfc.(string))
	}
	//BasePaths = append(BasePaths, []string(basePaths)...)
	log.Printf("Base paths: %v", BasePaths)
    

	// Deal with the hash (do we need it, how do we run it, and do we send it somewhere?)
	requireHash := viper.GetBool("hash.required")
	hashCmd := viper.GetString("hash.command")
	hashOpt := viper.GetString("hash.cmd-opt")

	hashRoutingKey := viper.GetString("hash.send-to")
	sendHash := false
	var hashMessage P8Message
	if len(hashRoutingKey) > 0 {
		if AmqpSenderIsActive == false {
			// sometimes the AMQP sender takes a little time to startup, so wait a second
			var waitTime int = 1
			if viper.IsSet("classifier.wait-for-sender") {
				waitTime = viper.GetInt("classifier.wait-for-sender")
			}
			time.Sleep(time.Duration(waitTime) * time.Second)
			if AmqpSenderIsActive == false {
				log.Printf("[classifier] Cannot start classifier because the AMQP sender routine is not active, and sending hashes has been requested")
				context.ReqQueue <- ThreadCannotContinue
				return
			}
		}
		sendHash = true
		hashMessage = PrepareRequest([]string{hashRoutingKey}, "application/msgpack", MOCommand, nil)
		payload := make(map[string]interface{})
		payload["values"] = []string{"do_insert"}
		payload["file_name"] = ""
		payload["file_hash"] = ""
		payload["run_id"] = 106
		hashMessage.Payload = payload
	}

	log.Print("[classifier] started successfully")

classifierLoop:
	for {
		select {
		// the control messages can stop execution
		case controlMsg := <-context.CtrlQueue:
			if controlMsg == StopExecution {
				log.Print("[classifier] stopping on interrupt.")
				break classifierLoop
			}
		case fileHeader := <-context.FileStream:
			inputFilePath := filepath.Join(fileHeader.HotPath, fileHeader.Filename)
			opReturn := OperatorReturn{
				Operator: "shipper",
				FHeader:  fileHeader,
				Err:      nil,
				IsFatal:  false,
			}

			if _, existsErr := os.Stat(inputFilePath); os.IsNotExist(existsErr) {
				opReturn.Err = fmt.Errorf("[classifier] file <%s> does not exist", inputFilePath)
				opReturn.IsFatal = true
				log.Printf(opReturn.Err.Error())
				context.RetStream <- opReturn
				break
			}

			_, inputFilename := filepath.Split(inputFilePath)

			acceptType := bool(false)

typeLoop:
			for _, typeInfo := range types {
				acceptType = true // this must start as true for this multi-test setup to work
				if typeInfo.DoMatchExtension {
					acceptType = acceptType && strings.HasSuffix(inputFilename, typeInfo.Extension)
				}
				if typeInfo.DoMatchRegexp {
					acceptType = acceptType && typeInfo.RegexpTemplate.MatchString(inputFilename)
				}

				if acceptType {
					log.Printf("[classifier] Classifying file <%s> as type <%s>", inputFilename, typeInfo.Name)
					opReturn.FHeader.FileType = typeInfo.Name
					opReturn.FHeader.SubPath = getSubPath(opReturn.FHeader.HotPath)
					opReturn.FHeader.DoNearline = typeInfo.DoNearline
					opReturn.FHeader.SetNearlineCmd(typeInfo.NearlineCmd)
					if typeInfo.DoHash {
						if hash, hashErr := exec.Command(hashCmd, hashOpt, inputFilePath).CombinedOutput(); hashErr != nil {
							opReturn.Err = fmt.Errorf("[classifier] error while hashing:\n\t%v", hashErr.Error())
							opReturn.IsFatal = requireHash
							log.Printf(opReturn.Err.Error())
						} else {
							hashTokens := strings.Fields(string(hash))
							opReturn.FHeader.FileHash = hashTokens[0]
							log.Printf("[classifier] file <%s> hash: %s", inputFilename, opReturn.FHeader.FileHash)
							if sendHash {
								hashMessage.TimeStamp = time.Now().UTC().Format(TimeFormat)
								hashMessage.Payload.(map[string]interface{})["file_name"] = inputFilename
								hashMessage.Payload.(map[string]interface{})["file_hash"] = opReturn.FHeader.FileHash
								//hashMessage.Payload.(map[string]interface{})["run_id"] = ???
								//log.Printf("[classifier] Sending hash message:\n\t%v", hashMessage)
								SendMessageQueue <- hashMessage
							}
						}
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
}
