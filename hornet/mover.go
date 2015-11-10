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
	"os"
	"path/filepath"
	"time"

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

	tempDest := destination + ".hmtemp"
	dst, dstErr := os.Create(tempDest)
	if dstErr != nil {
		return dstErr
	}
	// we can't defer the Close() call because we need to rename after the close
	//defer dst.Close()

	if _, cpyErr := io.Copy(dst, src); cpyErr != nil {
		dst.Close()
		return cpyErr
	}
	dst.Close() // this has to be done before calling Rename()

	if renameErr := os.Rename(tempDest, destination); renameErr != nil {
		return renameErr
	}
	if chmodErr := os.Chmod(destination, 0664); chmodErr != nil {
		return chmodErr
	}

	return nil
}

// Copy copies a file from one place to another.
func Copy(src, dest string) (e error) {
	if copyErr := copy(src, dest); copyErr != nil {
		Log.Error("File copy failed! (%v -> %v) [%v]\n", src, dest, copyErr)
		e = errors.New("Failed to copy file")
		if remErr := Remove(dest); remErr != nil {
			Log.Error("Failed to remove the failed-copy (destination) file [%v]", remErr)
			e = errors.New("Failed to copy file & failed to removed the failed-copy destination file")
		}
	}
	return
}

// Remove deletes a file
func Remove(file string) (e error) {
	if rmErr := os.Remove(file); rmErr != nil {
		Log.Error("File rm failed! (%v) [%v]\n", file, rmErr)
		e = errors.New("Failed to remove file")
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
	defer Log.Info("Mover is finished.")

	// keep a running list of all of the directories we know about.
	ds := make(DirectorySet)

	destDirBase, dirErr := filepath.Abs(viper.GetString("mover.dest-dir"))
	if dirErr != nil || PathIsDirectory(destDirBase) == false{
		Log.Critical("Destination directory is not valid: <%v>", destDirBase)
		context.ReqQueue <- ThreadCannotContinue
		return
	}

	Log.Info("Mover started successfully")

moveLoop:
	for {
		select {
		// the control messages can stop execution
		// TODO: should finish pending jobs before dying.
		case controlMsg, queueOk := <-context.CtrlQueue:
			if ! queueOk {
				Log.Error("Control queue has closed unexpectedly")
				break moveLoop
			}
			if controlMsg == StopExecution {
				Log.Info("Mover stopping on interrupt.")
				break moveLoop
			}
		case fileHeader, queueOk := <-context.FileStream:
			if ! queueOk {
				Log.Error("File stream has closed unexpectedly")
				context.ReqQueue <- StopExecution
				break moveLoop
			}
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
				Log.Info("Creating/adding directory %s\n", destDirPath)
				if mkErr := os.MkdirAll(destDirPath, os.ModeDir|os.ModePerm); mkErr != nil {
					opReturn.Err = fmt.Errorf("Couldn't make directory %v: [%v]", destDirPath, mkErr)
					opReturn.IsFatal = true
					Log.Error(opReturn.Err.Error())
				} else {
					ds[destDirPath] = true
				}
			}

			deleteInputFile := true

			// copy the file
			timeStart := time.Now()
			if copyErr := Copy(inputFilePath, outputFilePath); copyErr != nil {
				opReturn.Err = fmt.Errorf("Error copying (%v -> %v) [%v]", inputFilePath, outputFilePath, copyErr)
				opReturn.IsFatal = true
				Log.Error(opReturn.Err.Error())
				deleteInputFile = false
			} else if len(opReturn.FHeader.FileHash) > 0 {
				if hash, hashErr := Hash(inputFilePath); hashErr != nil {
					opReturn.Err = hashErr
					opReturn.IsFatal = true
					Log.Error(opReturn.Err.Error())
					deleteInputFile = false
				} else if opReturn.FHeader.FileHash != hash {
					opReturn.Err = fmt.Errorf("Warm and hot copies of the file do not match!\n\tInput: %s\n\tOutput: %s", inputFilePath, outputFilePath)
					opReturn.IsFatal = true
					Log.Error(opReturn.Err.Error())
					deleteInputFile = false
				}
				// else: hash matches, so deleting the input file is ok
			}
			timeEnd := time.Now()
			if fileInfo, fiErr := os.Stat(outputFilePath); fiErr != nil {
				Log.Warning("Unable to get file information on the output file: %v", fiErr)
				Log.Info("File copy took %v to transfer [unknown] bytes", timeEnd.Sub(timeStart))
			} else {
				Log.Info("File copy took %v to transfer %v bytes", timeEnd.Sub(timeStart), fileInfo.Size())
			}

			if deleteInputFile == true {
				if rmErr := Remove(inputFilePath); rmErr != nil {
					opReturn.Err = fmt.Errorf("Error removing file %s", inputFilePath)
					opReturn.IsFatal = true
					Log.Error(opReturn.Err.Error())
				}
			}

			context.RetStream <- opReturn
		}

	}
}
