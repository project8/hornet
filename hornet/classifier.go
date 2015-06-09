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
	"text/template"
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
	//DoNearline       bool
	//NearlineCmd      string
	Jobs             []int
}

type JobInfo struct {
	Name             string
	FileType         string
	Command          string
	CommandTemplate  *template.Template
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
		log.Printf("[classifier] Adding type:\n\t%v", types[iType])
	}

	// Process the jobs
	maxJobs := uint(viper.GetInt("classifier.max-jobs"))

	jobsRawIfc :=  viper.Get("workers.jobs")
	jobsRaw := jobsRawIfc.([]interface{})

	var jobs = make([]JobInfo, len(jobsRaw))
	var cmdErr error
	for iJob, jobMapIfc := range jobsRaw {
		jobMap := jobMapIfc.(map[string](interface{}))
		jobs[iJob].Name = jobMap["name"].(string)
		jobs[iJob].FileType = jobMap["file-type"].(string)
		jobs[iJob].Command = jobMap["command"].(string)
		if jobs[iJob].CommandTemplate, cmdErr = template.New("cmd").Parse(jobs[iJob].Command); cmdErr != nil {
			log.Printf("[classifier] template error while processing <%v>", jobs[iJob].Command)
			context.CtrlQueue <- ThreadCannotContinue;
			return
		}
		log.Printf("[classifier] Adding job:\n\t%v", jobs[iJob])

		// add this job to the list of jobs for its file type
		for iType, _ := range types {
			if types[iType].Name == jobs[iJob].FileType {
				types[iType].Jobs = append(types[iType].Jobs, iJob)
				log.Printf("[classifier] type <%s> will now perform job %d <%s>: %v", types[iType].Name, iJob, jobs[iJob].Name, types[iType].Jobs)
			}
		}
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

	// Sending the file info
	sendtoRoutingKey := viper.GetString("classifier.send-to")
	sendFileInfo := viper.GetBool("classifier.send-file-info")
	var masterFileInfoMessage P8Message
	if sendFileInfo {
		if AmqpSenderIsActive == false {
			// sometimes the AMQP sender takes a little time to startup, so wait a second
			var waitTime int = 1
			if viper.IsSet("classifier.wait-for-sender") {
				waitTime = viper.GetInt("classifier.wait-for-sender")
			}
			time.Sleep(time.Duration(waitTime) * time.Second)
			if AmqpSenderIsActive == false {
				log.Printf("[classifier] Cannot start classifier because the AMQP sender routine is not active, and sending file info has been requested")
				context.ReqQueue <- ThreadCannotContinue
				return
			}
		}
		masterFileInfoMessage = PrepareRequest([]string{sendtoRoutingKey}, "application/msgpack", MOCommand, nil)
		masterFileInfoMessage.Payload = make(map[string]interface{})
		masterFileInfoMessage.Payload.(map[string]interface{})["values"] = []string{"do_insert"}
		masterFileInfoMessage.Payload.(map[string]interface{})["file_name"] = ""
		masterFileInfoMessage.Payload.(map[string]interface{})["file_hash"] = ""
		masterFileInfoMessage.Payload.(map[string]interface{})["run_id"] = 0
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
			var fileInfoMessage P8Message
			if sendFileInfo {
				fileInfoMessage = masterFileInfoMessage
			}

typeLoop:
			for _, typeInfo := range types {
				acceptType = true // this must start as true for this multi-test setup to work
				if typeInfo.DoMatchExtension {
					acceptType = acceptType && strings.HasSuffix(inputFilename, typeInfo.Extension)
				}
				if typeInfo.DoMatchRegexp {
					allSubmatches := typeInfo.RegexpTemplate.FindAllStringSubmatch(inputFilename, -1)
					acceptType = acceptType && len(allSubmatches) == 1 && len(allSubmatches[0]) > 1 && allSubmatches[0][0] == inputFilename
					if acceptType && sendFileInfo {
						subexpNames := typeInfo.RegexpTemplate.SubexpNames()
						if len(allSubmatches[0]) > 1 {
							for iSubmatch, submatch := range allSubmatches[0][1:] {
								log.Printf("[classifier] adding to payload: %s: %s", subexpNames[iSubmatch+1], submatch)
								fileInfoMessage.Payload.(map[string]interface{})[subexpNames[iSubmatch+1]] = submatch
							}
						}
					}
				}

				if acceptType {
					log.Printf("[classifier] Classifying file <%s> as type <%s>", inputFilename, typeInfo.Name)
					opReturn.FHeader.FileType = typeInfo.Name
					opReturn.FHeader.SubPath = getSubPath(opReturn.FHeader.HotPath)
					opReturn.FHeader.JobQueue = make(chan Job, maxJobs)
					if typeInfo.DoHash {
						if hash, hashErr := exec.Command(hashCmd, hashOpt, inputFilePath).CombinedOutput(); hashErr != nil {
							opReturn.Err = fmt.Errorf("[classifier] error while hashing:\n\t%v", hashErr.Error())
							opReturn.IsFatal = requireHash
							log.Printf(opReturn.Err.Error())
						} else {
							hashTokens := strings.Fields(string(hash))
							opReturn.FHeader.FileHash = hashTokens[0]
							log.Printf("[classifier] file <%s> hash: %s", inputFilename, opReturn.FHeader.FileHash)
						}
					}
					if sendFileInfo {
						fileInfoMessage.TimeStamp = time.Now().UTC().Format(TimeFormat)
						fileInfoMessage.Payload.(map[string]interface{})["file_name"] = inputFilename
						fileInfoMessage.Payload.(map[string]interface{})["file_hash"] = opReturn.FHeader.FileHash
						SendMessageQueue <- fileInfoMessage
					}
					// jobs for the job queue
					log.Printf("[classifier] type %s has %d jobs: %v", typeInfo.Name, len(typeInfo.Jobs), typeInfo.Jobs)
					for _, jobId := range typeInfo.Jobs {
						newJob := Job{
							//Command: jobs[jobId].Command,
							CommandTemplate: jobs[jobId].CommandTemplate,
						}
						
						select {
						case opReturn.FHeader.JobQueue <- newJob:
							// do nothing
						default:
							log.Printf("[classifier] attempting to submit more than the maximum number of jobs for a file; aborting")
							context.CtrlQueue <- ThreadCannotContinue
							break classifierLoop
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
