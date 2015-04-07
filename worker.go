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

	// the base command we're going to run.  everything is fixed at build time
	// except for the input and output names.  we mutate the array in place
	// we get events, so we keep the indices to those elements in the argument
	// array.
	cmd := exec.Command(config.KatydidPath,
		"-c",
		config.KatydidConfPath,
		"-e",
		"dummyInName",
		"--hdf5-file",
		"dummyOutName")
	inFnamePos := len(cmd.Args) - 3
	outFnamePos := len(cmd.Args) - 1

	var jobCount JobID
	for f := range context.FilePipeline {
		cmd.Args[inFnamePos] = f
		cmd.Args[outFnamePos] = fmt.Sprintf("%s_%d_%d.h5", f, id, jobCount)

		// run the process
		if stdout, stdoutErr := cmd.StdoutPipe(); stdoutErr == nil {
			var startTime time.Time
			if procErr := cmd.Start(); procErr != nil {
				localLog(jobCount,
					"couldn't start command: %v", procErr)
			} else {
				startTime = time.Now()
				output, readErr := ioutil.ReadAll(stdout)
				if readErr != nil {
					localLog(jobCount,
						"Error running process: %v",
						string(output[:]))
				}
			}
			cmd.Wait()
			localLog(jobCount,
				"Execution finished.  Elapsed time: %v",
				time.Since(startTime))

		} else {
			localLog(jobCount, "error opening stdout: %v", stdoutErr)
		}
		jobCount++
	}
	localLog(jobCount+1,
		"no work remaining.  total of %d jobs processed.",
		jobCount)
}
