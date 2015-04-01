/*
* worker.go
 */
package main

import (
	"io/ioutil"
	"log"
	"os/exec"
	"time"
)

// Worker waits for strings on a channel, and launches a Katydid process for
// each string it receives, which should be the name of the file to process.
func Worker(context Context, config Config) {
	// Put off being done until there's nothing left in the channel
	// to process
	defer context.Pool.Done()

	// a little helpful closure over the variables we already know
	buildCmd := func(fname string) *exec.Cmd {
		return exec.Command(config.KatydidPath,
			"-c",
			config.KatydidConfPath,
			"-e",
			fname)
	}

	for f := range context.FilePipeline {
		// build the command, using the new filename.
		// FIXME: this will allocate some memory, can we avoid that?
		// probably...
		cmd := buildCmd(f)

		// run the process
		if stdout, stdoutErr := cmd.StdoutPipe(); stdoutErr == nil {
			var startTime time.Time
			if procErr := cmd.Start(); procErr != nil {
				log.Printf("couldn't start command: %v", procErr)
			} else {
				startTime = time.Now()
				output, readErr := ioutil.ReadAll(stdout)
				if readErr != nil {
					log.Printf("Error running process: %v",
						string(output[:]))
				}
			}
			cmd.Wait()
			log.Printf("Execution finished.  Elapsed time: %v\n",
				time.Since(startTime))

		} else {
			log.Printf("error opening stdout: %v", stdoutErr)
		}

	}
}
