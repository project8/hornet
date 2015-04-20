/*
* mover.go
*
* the mover thread moves files from a source location to a destination.
* it can batch files for moving if requested.
 */
package main

import (
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
)

type DirectorySet map[string]bool

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

// Move moves a file from one place to another.  This is equivalent to
// copy-and-delete.
func Move(src, dest string) (e error) {
	if copyErr := copy(src, dest); copyErr != nil {
		log.Printf("file copy failed! (%v -> %v) [%v]\n",
			src, dest, copyErr)
		e = errors.New("failed to copy file")
	} else {
		if rmErr := os.Remove(src); rmErr != nil {
			log.Printf("file rm failed! (%v) [%v]\n",
				src,
				rmErr)
			e = errors.New("failed to remove old file")
		}
	}

	return
}

// Mover receives filenames over an unbuffered channel, and moves them from
// their current place on the filesystem to a destination.  If so configured
// it will wait (up to a timeout) until it has some number of files to move
// in a batch.  It is stopped when it receives a message from the main thread
// to shut down.
func Mover(context Context, config Config) {
	// decrement the wg counter at the end
	defer context.Pool.Done()

	// keep a running list of all of the directories we know about.
	ds := make(DirectorySet)

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
			destName, destErr := RenamePathRelativeTo(newFile,
				config.WatchDirPath,
				config.DestDirPath)
			if destErr != nil {
				log.Printf("bad rename request: %s -> %s w.r.t %s\n",
					newFile, config.DestDirPath, config.WatchDirPath)
			} else {
				// check if we already know about the destdir
				newDir := filepath.Dir(destName)
				if ds[newDir] == false {
					if mkErr := os.MkdirAll(newDir, os.ModeDir); mkErr != nil {
						log.Printf("couldn't make directory %v: [%v]",
							newDir, mkErr)
					}
					copy(newFile, destName)
				}
				if moveErr := Move(newFile, destName); moveErr != nil {
					log.Printf("error moving (%v -> %v) [%v]",
						newFile, destName, moveErr)
				}
			}

		}

	}

	// Finish any pending move jobs.

	log.Print("mover finished.")
}
