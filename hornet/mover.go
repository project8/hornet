/*
* mover.go
*
* the mover thread moves files from a source location to a destination.
* it can batch files for moving if requested.
 */
package hornet

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

/*
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
*/

// Copy copies a file from one place to another.
func Copy(src, dest string) (e error) {
	if copyErr := copy(src, dest); copyErr != nil {
		log.Printf("[mover] file copy failed! (%v -> %v) [%v]\n",
			src, dest, copyErr)
		e = errors.New("failed to copy file")
	}
	return
}

// Remove deletes a file
func Remove(file string) (e error) {
	if rmErr := os.Remove(file); rmErr != nil {
		log.Printf("[mover] file rm failed! (%v) [%v]\n", file, rmErr)
		e = errors.New("failed to remove  file")
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
	defer log.Print("[mover] finished.")

	// keep a running list of all of the directories we know about.
	ds := make(DirectorySet)

	destDirBase, dirErr := filepath.Abs(viper.GetString("mover.dest-dir"))
	if dirErr != nil || PathIsDirectory(destDirBase) == false{
		log.Print("[mover] Destination directory is not valid: <%v>", destDirBase)
		context.ReqQueue <- ThreadCannotContinue
		return
	}

	// Deal with the hash (do we need it, how do we run it, and do we send it somewhere?)
	requireHash := viper.GetBool("hash.required")
	hashCmd := viper.GetString("hash.command")
	hashOpt := viper.GetString("hash.cmd-opt")

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
			inputFilePath := filepath.Join(fileHeader.HotPath, fileHeader.Filename)
			opReturn := OperatorReturn{
				Operator: "mover",
				FHeader:  fileHeader,
				Err:      nil,
				IsFatal:  false,
			}
			destDirPath := filepath.Clean(filepath.Join(destDirBase, fileHeader.SubPath))
			outputFilePath := filepath.Join(destDirPath, fileHeader.Filename)
			opReturn.FHeader.WarmPath = destDirPath
			opReturn.FHeader.FileWarmPath = outputFilePath
			// check if we already know about the destDirPath
			if ds[destDirPath] == false {
				log.Printf("[mover] creating directory %s\n", destDirPath)
				if mkErr := os.MkdirAll(destDirPath, os.ModeDir|os.ModePerm); mkErr != nil {
					opReturn.Err = fmt.Errorf("[mover] couldn't make directory %v: [%v]", destDirPath, mkErr)
					opReturn.IsFatal = true
					log.Printf(opReturn.Err.Error())
				} else {
					ds[destDirPath] = true
				}
			}
			// copy the file
			if copyErr := Copy(inputFilePath, outputFilePath); copyErr != nil {
				opReturn.Err = fmt.Errorf("[mover] error copying (%v -> %v) [%v]", inputFilePath, outputFilePath, copyErr)
				opReturn.IsFatal = true
				log.Printf(opReturn.Err.Error())
			}

			if len(opReturn.FHeader.FileHash) > 0 {
				if hash, hashErr := exec.Command(hashCmd, hashOpt, inputFilePath).CombinedOutput(); hashErr != nil {
					opReturn.Err = fmt.Errorf("[mover] error while hashing:\n\t%v", hashErr.Error())
					opReturn.IsFatal = requireHash
					log.Printf(opReturn.Err.Error())
				} else {
					if opReturn.FHeader.FileHash != strings.Fields(string(hash))[0] {
						opReturn.Err = fmt.Errorf("[mover] warm and hot copies of the file do not match!\n\tInput: %s\n\tOutput: %s", inputFilePath, outputFilePath)
						opReturn.IsFatal = true
						log.Printf(opReturn.Err.Error())
					} else {
						// files match; ok to delete the input file
						if rmErr := Remove(inputFilePath); rmErr != nil {
							opReturn.Err = fmt.Errorf("[mover] error removing file %s", inputFilePath)
							opReturn.IsFatal = true
							log.Printf(opReturn.Err.Error())
						}
					}
				}
			}

			context.RetStream <- opReturn
		}

	}
}
