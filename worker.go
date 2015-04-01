/*
* worker.go
*/
package main

import (
	"log"
)

// Worker waits for strings on a channel, and launches a Katydid process for
// each string it receives, which should be the name of the file to process.
func Worker(context Context, config Config) {
	// Put off being done until there's nothing left in the channel
	// to process
	defer context.Pool.Done()

	for f := range context.FilePipeline {
		log.Print(f)
	}
}
