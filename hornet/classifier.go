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
		e = errors.New("No types were provided")
		Log.Critical(e.Error())
		return
	}
	typesRaw := typesRawIfc.([]interface{})

	//var types = make([]TypeInfo, len(typesRaw))
	nTestsPresent := uint(0)
	for iType, typeMapIfc := range typesRaw {
		typeMap := typeMapIfc.(map[string](interface{}))
		if typeMap["name"].(string) == "" {
			e = fmt.Errorf("Type %d is missing its name", iType)
			Log.Critical(e.Error())
		}
		if _, hasExt := typeMap["match-extension"]; hasExt {
			nTestsPresent++
		}
		if regexpTemplate, hasRegexp := typeMap["match-regexp"]; hasRegexp {
			nTestsPresent++
			if _, regexpErr := regexp.Compile(regexpTemplate.(string)); regexpErr != nil {
				e = fmt.Errorf("Invalid regular expression: %s\n\t%v", regexpTemplate.(string), regexpErr.Error())
				Log.Critical(e.Error())
			}
		}
		if nTestsPresent == 0 {
			e = fmt.Errorf("No tests are present for type %d", iType)
			Log.Critical(e.Error())
		}
	}

	if viper.IsSet("classifier.base-paths") {
		basePathsRawIfc := viper.Get("classifier.base-paths")
		basePathsRaw := basePathsRawIfc.([]interface{})
		for _, pathIfc := range basePathsRaw {
			if _, fpErr := filepath.Abs(pathIfc.(string)); fpErr != nil {
				e = fmt.Errorf("Invalid base path: <%v>", pathIfc.(string))
				Log.Critical(e.Error())
			}
		}
	}

	if viper.IsSet("hash") == false {
		e = errors.New("Hash configuration not provided")
		Log.Critical(e.Error())
		return
	}
	return
}

// getSubPath extracts the sub path from a file's full path by comparing it to the (ordered) list of base paths
func getSubPath(path string) (subPath string) {
	subPath = ""
	for _, basePath := range BasePaths {
		if strings.HasPrefix(path, basePath) {
			var relErr error
			subPath, relErr = filepath.Rel(basePath, path)
			if relErr != nil {
				Log.Debug("Unable to get relative path after checking prefix:\n\t%s\n\t%s\n\t%v", basePath, path, relErr)
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
	defer Log.Info("Classifier is finished.")

	if configErr := ValidateClassifierConfig(); configErr != nil {
		Log.Critical("Error in the classifier configuration: %s", configErr.Error())
		context.ReqQueue <- ThreadCannotContinue
		return
	}

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
		Log.Info("Adding type:\n\t%v", types[iType])
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
			Log.Critical("Template error while processing <%v>", jobs[iJob].Command)
			context.ReqQueue <- ThreadCannotContinue;
			return
		}
		Log.Debug("Adding job:\n\t%v", jobs[iJob])

		// add this job to the list of jobs for its file type
		for iType, _ := range types {
			if types[iType].Name == jobs[iJob].FileType {
				types[iType].Jobs = append(types[iType].Jobs, iJob)
				Log.Info("Type <%s> will now perform job %d <%s>: %v", types[iType].Name, iJob, jobs[iJob].Name, types[iType].Jobs)
			}
		}
	}

	// Process the base paths
	BasePaths = make([]string, 0)

	if viper.GetBool("watcher.active") {
		if viper.IsSet("watcher.dir") {
			watchDir := viper.GetString("watcher.dir")
			if PathIsDirectory(watchDir) {
				watchDirAbs, _ := filepath.Abs(watchDir)
				BasePaths = append(BasePaths, watchDirAbs)
			}
		}
		if viper.IsSet("watcher.dirs") {
			watchDirs := viper.GetStringSlice("watcher.dirs")
			for _, watchDir := range watchDirs {
				if PathIsDirectory(watchDir) {
					watchDirAbs, _ := filepath.Abs(watchDir)
					BasePaths = append(BasePaths, watchDirAbs)
				}
			}
		}
	}

	basePathsRawIfc := viper.Get("classifier.base-paths")
	basePathsRaw := make([]interface{}, 0)
	if basePathsRawIfc != nil {
		basePathsRaw = basePathsRawIfc.([]interface{})
		for _, pathIfc := range basePathsRaw {
			basePathAbs, _ := filepath.Abs(pathIfc.(string))
			BasePaths = append(BasePaths, basePathAbs)
		}
	}

	//BasePaths = append(BasePaths, []string(basePaths)...)
	Log.Info("Base paths: %v", BasePaths)
    

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
				Log.Critical("Cannot start classifier because the AMQP sender routine is not active, and sending file info has been requested")
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

	Log.Info("Classifier started successfully")

classifierLoop:
	for {
		select {
		// the control messages can stop execution
		case controlMsg := <-context.CtrlQueue:
			if controlMsg == StopExecution {
				Log.Info("Classifier stopping on interrupt.")
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
				Log.Critical(opReturn.Err.Error())
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
								subexpName := subexpNames[iSubmatch+1]
								if len(subexpName) > 0 {
									Log.Debug("Adding to payload: %s: %s", subexpName, submatch)
									fileInfoMessage.Payload.(map[string]interface{})[subexpName] = submatch
								}
							}
						}
					}
				}

				if acceptType {
					Log.Info("Classifying file <%s> as type <%s>", inputFilename, typeInfo.Name)
					opReturn.FHeader.FileType = typeInfo.Name
					opReturn.FHeader.SubPath = getSubPath(opReturn.FHeader.HotPath)
					opReturn.FHeader.JobQueue = make(chan Job, maxJobs)
					if typeInfo.DoHash {
						if hash, hashErr := exec.Command(hashCmd, hashOpt, inputFilePath).CombinedOutput(); hashErr != nil {
							opReturn.Err = fmt.Errorf("[classifier] error while hashing:\n\t%v", hashErr.Error())
							opReturn.IsFatal = requireHash
							Log.Error(opReturn.Err.Error())
						} else {
							hashTokens := strings.Fields(string(hash))
							opReturn.FHeader.FileHash = hashTokens[0]
							Log.Debug("File <%s> hash: %s", inputFilename, opReturn.FHeader.FileHash)
						}
					}
					if sendFileInfo {
						fileInfoMessage.TimeStamp = time.Now().UTC().Format(TimeFormat)
						fileInfoMessage.Payload.(map[string]interface{})["file_name"] = inputFilename
						fileInfoMessage.Payload.(map[string]interface{})["file_hash"] = opReturn.FHeader.FileHash
						SendMessageQueue <- fileInfoMessage
					}
					// jobs for the job queue
					Log.Debug("Type %s has %d jobs: %v", typeInfo.Name, len(typeInfo.Jobs), typeInfo.Jobs)
					for _, jobId := range typeInfo.Jobs {
						newJob := Job{
							//Command: jobs[jobId].Command,
							CommandTemplate: jobs[jobId].CommandTemplate,
						}
						
						select {
						case opReturn.FHeader.JobQueue <- newJob:
							// do nothing
						default:
							Log.Critical("Attempting to submit more than the maximum number of jobs for a file; aborting")
							context.ReqQueue <- ThreadCannotContinue
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
				Log.Error("Unable to classify file <%s>", inputFilename)
				opReturn.Err = errors.New("[Classifier] Unable to classify")
				opReturn.IsFatal = true
				context.RetStream <- opReturn
			}
		}

	}
}
