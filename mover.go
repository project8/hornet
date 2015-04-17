/*
* mover.go
*
* the mover thread moves files from a source location to a destination.
* it can batch files for moving if requested.
 */
package main

import (
	"io"
	"log"
	"os"
)

// copy will copy the contents of one file to another.  the arguments are both
// strings i.e. paths to the original and the desired destination.  if something
// goes wrong, it returns an error.
func copy(source, destination string) error {
	src, srcErr := os.Open(source)
	if srcErr != nil {
		return srcErr
	}
	defer src.Close()

	dst, dstErr := os.Create(destination)
	if dstErr != nil {
		return dstErr
	}
	defer dst.Close()

	if _, cpyErr := io.Copy(dst, src); cpyErr != nil {
		return cpyErr
	}

	return nil
}

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
		case controlMsg := <-context.Control:
			if controlMsg == StopExecution {
				log.Print("mover stopping on interrupt.")
				break moveLoop
			}
		case newFile := <-context.FinishedFileStream:
			destName := MovedFilePath(newFile, config.DestDirPath)
			if copyErr := copy(newFile, destName); copyErr != nil {
				log.Printf("file copy failed! (%v -> %v) [%v]\n",
					newFile, destName, copyErr)
			} else {
				if rmErr := os.Remove(newFile); rmErr != nil {
					log.Printf("file rm failed! (%v) [%v]\n",
						newFile,
						rmErr)
				}
			}
		}

	}

	// Finish any pending move jobs.

	log.Print("mover finished.")
}
