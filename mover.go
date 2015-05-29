/*
* mover.go
*
* the mover thread moves files from a source location to a destination.
* it can batch files for moving if requested.
 */
package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// A DirectorySet is just a simple Set type for directories.
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
		log.Printf("[mover] file copy failed! (%v -> %v) [%v]\n",
			src, dest, copyErr)
		e = errors.New("failed to copy file")
	} else {
		if rmErr := os.Remove(src); rmErr != nil {
			log.Printf("[mover] file rm failed! (%v) [%v]\n",
				src,
				rmErr)
			e = errors.New("failed to remove old file")
		}
	}

	return
}

// Mover receives filenames over an unbuffered channel, and moves them from
// their current place on the filesystem to a destination.
// It is stopped when it receives a message from the main thread
// to shut down.
func Mover(context OperatorContext) {
	// decrement the wg counter at the end
	defer context.PoolCount.Done()

	// keep a running list of all of the directories we know about.
	ds := make(DirectorySet)

	watchDirPath := viper.GetString("watcher.dir")
	destDirPath := viper.GetString("mover.dest-dir")

	log.Print("[mover] started successfully")

moveLoop:
	for {
		select {
		// the control messages can stop execution
		// TODO: should finish pending jobs before dying.
		case controlMsg := <-context.CtrlQueue:
			if controlMsg == StopExecution {
				log.Print("[mover] stopping on interrupt.")
				break moveLoop
			}
		case fileHeader := <-context.FileStream:
			inputFile := filepath.Join(fileHeader.HotPath, fileHeader.Filename)
			opReturn := OperatorReturn{
				Operator: "mover",
				FHeader:  fileHeader,
				Err:      nil,
				IsFatal:  false,
			}
			outputFile, destErr := RenamePathRelativeTo(inputFile, watchDirPath, destDirPath)
			if destErr != nil {
				opReturn.Err = fmt.Errorf("[mover] bad rename request: %s -> %s w.r.t %s\n",
					inputFile, destDirPath, watchDirPath)
				opReturn.IsFatal = true
				log.Printf(opReturn.Err.Error())
			} else {
				outputPath, _ := filepath.Split(outputFile)
				opReturn.FHeader.WarmPath = outputPath
				// check if we already know about the destdir
				newDir := filepath.Dir(outputFile)
				if ds[newDir] == false {
					log.Printf("[mover] creating directory %s\n", newDir)
					if mkErr := os.MkdirAll(newDir, os.ModeDir|os.ModePerm); mkErr != nil {
						opReturn.Err = fmt.Errorf("[mover] couldn't make directory %v: [%v]", newDir, mkErr)
						opReturn.IsFatal = true
						log.Printf(opReturn.Err.Error())
					} else {
						ds[newDir] = true
					}
				}
				if moveErr := Move(inputFile, outputFile); moveErr != nil {
					opReturn.Err = fmt.Errorf("[mover] error moving (%v -> %v) [%v]",
						inputFile, outputFile, moveErr)
					opReturn.IsFatal = true
					log.Printf(opReturn.Err.Error())
				}
			}
			context.RetStream <- opReturn
		}

	}

	// Finish any pending move jobs.

	log.Print("[mover] finished.")
}
