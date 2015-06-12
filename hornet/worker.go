/*
* worker.go
 */
package hornet

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"
	"time"

	//"github.com/spf13/viper"
)

// WorkerID is an identifier for a particular worker goroutine.
type WorkerID uint

// JobID is an identifier for a particular processing job.
type JobID uint

// Worker waits for strings on a channel, and launches a Katydid process for
// each string it receives, which should be the name of the file to process.
func Worker(context OperatorContext, id WorkerID) {
	// Put off being done until there's nothing left in the channel
	// to process
	defer context.PoolCount.Done()

	Log.Info("Worker (%d) started.  waiting for work...", id)

	var jobCount JobID

	// adds worker state to the beginning of a format string, for use with logging
	withState := func(format string) string {
		return fmt.Sprintf("[worker %d.%d] %s", id, jobCount, format)
	}

	//	for inputFile := range context.FileStream {
workLoop:
	for {
		select {
		// the control messages can stop execution
		// TODO: should finish pending jobs before dying.
		case controlMsg := <-context.CtrlQueue:
			if controlMsg == StopExecution {
				//localLog(jobCount, "stopping on interrupt.")
				Log.Notice(withState("stopping on interrupt."))
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

jobLoop:
			for {
				// select statement to make a non-blocking receive on the job queue channel
				select {
				case job := <-opReturn.FHeader.JobQueue: // get the job
					// execute parsing on job.Command
					var cmdBuf bytes.Buffer
					job.CommandTemplate.Execute(&cmdBuf, opReturn.FHeader)
					command := cmdBuf.String()
					// split parsed command into cmd name and args
					commandParts := strings.Fields(command)
					job.CommandName = commandParts[0]
					job.CommandArgs = commandParts[1:len(commandParts)]
					Log.Info(withState("Executing command: %s %v"), job.CommandName, job.CommandArgs)
					// create the command
					cmd := exec.Command(job.CommandName, job.CommandArgs...)

					// run the process
					var outputBytes []byte
					var outputError error
					if stdout, stdoutErr := cmd.StdoutPipe(); stdoutErr == nil {
						var startTime time.Time
						if procErr := cmd.Start(); procErr != nil {
							opReturn.Err = fmt.Errorf("couldn't start command: %v", procErr)
							Log.Error(withState(opReturn.Err.Error()))
						} else {
							startTime = time.Now()
							outputBytes, outputError = ioutil.ReadAll(stdout)
							if outputError != nil {
								opReturn.Err = fmt.Errorf("Error running process: %v", string(outputBytes[:]))
								Log.Error(withState(opReturn.Err.Error()))
							}
						}
						if runErr := cmd.Wait(); runErr != nil {
							if exitErr, ok := runErr.(*exec.ExitError); !ok {
								opReturn.Err = fmt.Errorf("Nonzero exit status on process [%v].  Log: %v", exitErr, string(outputBytes[:]))
								Log.Error(withState(opReturn.Err.Error()))
							}
						}
						// if we're here, the job succeeded
						Log.Info(withState("Execution finished.  Elapsed time: %v"), time.Since(startTime))
						Log.Debug(withState("Job output:\n%s"), string(outputBytes))
						opReturn.FHeader.FinishedJobs = append(opReturn.FHeader.FinishedJobs, job)
					} else {
						opReturn.Err = fmt.Errorf("error opening stdout: %v", stdoutErr)
						Log.Error(withState(opReturn.Err.Error()))
					}
					jobCount++
				default: // non-blocking receive on the job queue
					Log.Info(withState("Finished processing jobs for this file"))
					context.RetStream <- opReturn
					break jobLoop
				} // end secondary select (non-blocking receive on the job queue
			} // end the job loop
		} // end main select
	} // end the worker loop
	Log.Info(withState("No work remaining.  Total of %d jobs processed."), jobCount)
}
