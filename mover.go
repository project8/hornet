/*
* mover.go
*
* the mover thread moves files from a source location to a destination.
* it can batch files for moving if requested.
 */
package main

import (
	"log"
)

// Mover receives filenames over an unbuffered channel, and moves them from
// their current place on the filesystem to a destination.  If so configured
// it will wait (up to a timeout) until it has some number of files to move
// in a batch.  It is stopped when it receives a message from the main thread
// to shut down.
func Mover(context Context, config Config) {
	defer context.Pool.Done()

moveLoop:
	for {
		select {
		// the control messages can stop execution
		// TODO: should finish pending jobs before dying.
		case controlMsg := <- context.Control:
			if controlMsg == StopExecution {
				log.Print("mover stopping on interrupt.")
				break moveLoop
			}
		case newFile := <- context.OutputFileStream:
			log.Printf("got file to move: %s\n", newFile)
		}
		
	}

	// Finish any pending move jobs.

	log.Print("mover finished.")
}
