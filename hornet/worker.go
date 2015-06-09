/*
* worker.go
 */
package hornet

import (
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	//"path/filepath"
	"strings"
	"time"

	//"github.com/spf13/viper"
)

// WorkerID is an identifier for a particular worker goroutine.
type WorkerID uint

// JobID is an identifier for a particular processing job.
type JobID uint

//func workerLog(format string, id WorkerID, job JobID, args ...interface{}) {
func workerLog(id WorkerID, job JobID, msg string) {
	//s := fmt.Sprintf(format, args)
	//log.Printf("[worker %d.%d] %s\n", id, job, s)
	log.Printf("[worker %d.%d] %s\n", id, job, msg)
}

// Worker waits for strings on a channel, and launches a Katydid process for
// each string it receives, which should be the name of the file to process.
func Worker(context OperatorContext, id WorkerID) {
	// Put off being done until there's nothing left in the channel
	// to process
	defer context.PoolCount.Done()

	// Close workerLog over known parameters
	//localLog := func(job JobID, format string, args ...interface{}) {
	localLog := func(job JobID, msg string) {
		//workerLog(format, id, job, args...)
		workerLog(id, job, msg)
	}
	log.Printf("[worker] worker (%d) started.  waiting for work...\n", id)

	var jobCount JobID
	//	for inputFile := range context.FileStream {
workLoop:
	for {
		select {
		// the control messages can stop execution
		// TODO: should finish pending jobs before dying.
		case controlMsg := <-context.CtrlQueue:
			if controlMsg == StopExecution {
				localLog(jobCount, "stopping on interrupt.")
				break workLoop
			}
		case fileHeader := <-context.FileStream:
			//inputFile := filepath.Join(fileHeader.WarmPath, fileHeader.Filename)
			opReturn := OperatorReturn{
				Operator: fmt.Sprintf("worker_%d", id),
				FHeader:  fileHeader,
				Err:      nil,
				IsFatal:  false,
			}

			// after moving the worker stage to after the mover stage, this is no longer necessary
			//opReturn.File, opReturn.Err := RenamePathRelativeTo(inputFile,
			//	config.WatchDirPath,
			//	config.DestDirPath)
			//if opReturn.Err != nil {
			//	log.Printf("[worker] bad rename request [%s -> %s] [%v]",
			//		config.WatchDirPath, config.DestDirPath, opReturn.Err)
			//}

			//outputFile := fmt.Sprintf("%s_%d_%d.h5", inputFile, id, jobCount)
			//opReturn.FHeader.AddSecondaryFile(outputFile)
			//localLog(jobCount, fmt.Sprintf("Executing command: %s %v", opReturn.FHeader.NearlineCmdName, opReturn.FHeader.NearlineCmdArgs))
			//cmd := exec.Command(opReturn.FHeader.NearlineCmdName, opReturn.FHeader.NearlineCmdArgs...)

			// select statement to make a non-blocking receive on the job queue channel
			select {
			case job := <-opReturn.FHeader.JobQueue: // get the job
				// execute parsing on job.Command
				// TODO: parse
				command := job.Command // temporary, w/ no parsing
				// split parsed command into cmd name and args
				commandParts := strings.Fields(command)
				job.CommandName = commandParts[0]
				job.CommandArgs = commandParts[1:len(commandParts)]
				localLog(jobCount, fmt.Sprintf("Executing command: %s %v", job.CommandName, job.CommandArgs))
				// create the command
				cmd := exec.Command(job.CommandName, job.CommandArgs...)
				// (after running) put completed job into FinishedJobs



			/*
				cmd := exec.Command(config.KatydidPath,
					"-c",
					config.KatydidConfPath,
					"-e",
					inputFile,
					"--hdf5-file",
					outputFile)
			*/
			// for debugging purposes (the println is to avoid an unused variable error for outputPath)
			//fmt.Println("InputFile:", inputFile)
			//fmt.Println("OutputFile:",outputFile)
			//cmd := exec.Command("ls")

				// run the process
				var outputBytes []byte
				var outputError error
				if stdout, stdoutErr := cmd.StdoutPipe(); stdoutErr == nil {
					var startTime time.Time
					if procErr := cmd.Start(); procErr != nil {
						opReturn.Err = fmt.Errorf("couldn't start command: %v", procErr)
						localLog(jobCount, opReturn.Err.Error())
					} else {
						startTime = time.Now()
						outputBytes, outputError = ioutil.ReadAll(stdout)
						if outputError != nil {
							opReturn.Err = fmt.Errorf("Error running process: %v", string(outputBytes[:]))
							localLog(jobCount, opReturn.Err.Error())
						}
					}
					if runErr := cmd.Wait(); runErr != nil {
						if exitErr, ok := runErr.(*exec.ExitError); !ok {
							opReturn.Err = fmt.Errorf("Nonzero exit status on process [%v].  Log: %v", exitErr, string(outputBytes[:]))
							localLog(jobCount, opReturn.Err.Error())
						}
					}
					// if we're here, the job succeeded
					localLog(jobCount, fmt.Sprintf("Execution finished.  Elapsed time: %v", time.Since(startTime)))
					localLog(jobCount, fmt.Sprintf("Job output:\n%s", string(outputBytes)))
					opReturn.FHeader.FinishedJobs = append(opReturn.FHeader.FinishedJobs, job)
				} else {
					opReturn.Err = fmt.Errorf("error opening stdout: %v", stdoutErr)
					localLog(jobCount, opReturn.Err.Error())
				}
				jobCount++
			default: // non-blocking receive on the job queue
				localLog(jobCount, "Finished processing jobs for this file")
				context.RetStream <- opReturn
			} // end secondary select (non-blocking receive on the job queue
		} // end main select
	} // end for
	localLog(jobCount, fmt.Sprintf("no work remaining.  total of %d jobs processed.", jobCount))
}
