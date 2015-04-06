/*
* worker.go
 */
package main

import (
	"io/ioutil"
	"log"
	"os/exec"
	"time"
	"fmt"
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
	
	// Put off being done until there's nothing left in the channel
	// to process
	defer context.Pool.Done()

	// a little helpful closure over the variables we already know
	buildCmd := func(fname string, job JobID) *exec.Cmd {
		outFilename := fmt.Sprintf("%d_%d.h5", id, job) 
		return exec.Command(config.KatydidPath,
			"-c",
			config.KatydidConfPath,
			"-e",
			fname,
			"--hdf5-file",
			outFilename)
	}
	
	var jobCount JobID = 0
	for f := range context.FilePipeline {
		// build the command, using the new filename.
		// FIXME: this will allocate some memory, can we avoid that?
		// probably...
		cmd := buildCmd(f, jobCount)

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
			localLog(jobCount,"error opening stdout: %v", stdoutErr)
		}
		jobCount++
	}
}
