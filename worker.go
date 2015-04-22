/*
* worker.go
 */
package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"time"
)

// WorkerID is an identifier for a particular worker goroutine.
type WorkerID uint

// JobID is an identifier for a particular processing job.
type JobID uint

func workerLog(format string, id WorkerID, job JobID, args ...interface{}) {
	s := fmt.Sprintf(format, args)
	log.Printf("[%d.%d] %s\n", id, job, s)
}

// Worker waits for strings on a channel, and launches a Katydid process for
// each string it receives, which should be the name of the file to process.
func Worker(context Context, config Config, id WorkerID) {
	// Close workerLog over known parameters
	localLog := func(job JobID, format string, args ...interface{}) {
		workerLog(format, id, job, args...)
	}
	log.Printf("worker (%d) started.  waiting for work...\n", id)

	// Put off being done until there's nothing left in the channel
	// to process
	defer context.Pool.Done()

	var jobCount JobID
	for f := range context.NewFileStream {
		outputPath, destErr := RenamePathRelativeTo(f,
			config.WatchDirPath,
			config.DestDirPath)
		if destErr != nil {
			log.Printf("bad rename request [%s -> %s] [%v]",
				config.WatchDirPath, config.DestDirPath, destErr)
		}
		cmd := exec.Command(config.KatydidPath,
			"-c",
			config.KatydidConfPath,
			"-e",
			f,
			"--hdf5-file",
			fmt.Sprintf("%s_%d_%d.h5", outputPath, id, jobCount))

		// run the process
		var outputBytes []byte
		var outputError error
		if stdout, stdoutErr := cmd.StdoutPipe(); stdoutErr == nil {
			var startTime time.Time
			if procErr := cmd.Start(); procErr != nil {
				localLog(jobCount,
					"couldn't start command: %v", procErr)
			} else {
				startTime = time.Now()
				outputBytes, outputError = ioutil.ReadAll(stdout)
				if outputError != nil {
					localLog(jobCount,
						"Error running process: %v",
						string(outputBytes[:]))
				}
			}
			if runErr := cmd.Wait(); runErr != nil {
				if exitErr, ok := runErr.(*exec.ExitError); !ok {
					log.Printf("Nonzero exit status on process [%v].  Log: %v",
						exitErr,
						string(outputBytes[:]))
				}
			}
			localLog(jobCount,
				"Execution finished.  Elapsed time: %v",
				time.Since(startTime))
			context.FinishedFileStream <- f

		} else {
			localLog(jobCount, "error opening stdout: %v", stdoutErr)
		}
		jobCount++
	}
	localLog(jobCount+1,
		"no work remaining.  total of %d jobs processed.",
		jobCount)
}
