package hornet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/fsnotify.v1"
)

func shouldPayAttention(evt fsnotify.Event) bool {
	// Returns true if the event was triggered by file creation or rename (i.e. move)
	newCreated := evt.Op == fsnotify.Create
	wasMovedTo := evt.Op == fsnotify.Rename
	return newCreated || wasMovedTo
}

func isEintr(e error) bool {
	// Detects particular system interrupt error
	return e != nil && strings.Contains(e.Error(), "interrupted system call")
}

func fileMoratorium(file string, fileQueue chan string, waitTime time.Duration) {
	time.Sleep(waitTime)
	fileQueue <- file
	return
}

func Watcher(context OperatorContext) {
	defer context.PoolCount.Done()
	defer Log.Info("Watcher is finished.")

	// New subdirectory watcher
	watcher, watcherErr := fsnotify.NewWatcher()
	if watcherErr != nil {
		Log.Criticalf("Could not create the watcher! %v", watcherErr)
		context.ReqQueue <- ThreadCannotContinue
		return
	}
	defer watcher.Close()

	moratoriumTime := 5 * time.Second
	if viper.IsSet("watcher.file-wait-time") {
		moratoriumTime = viper.GetDuration("watcher.file-wait-time")
	}
	Log.Debugf("File moratorium time: %v", moratoriumTime)

	ignoreDirs := []string{}
	if viper.IsSet("watcher.ignore-dirs") {
		ignoreDirs = viper.GetStringSlice("watcher.ignore-dirs")
	}
	Log.Infof("Ignoring directories named: %v", ignoreDirs)

	processRecursiveDir := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			Log.Criticalf("Unable to recursively process directory %s", path)
			procErr := fmt.Errorf("Unable to recursively process directory %s", path)
			return procErr
		}
		if PathIsRegularFile(path) {
			// File case
			Log.Debugf("Submitting file [%v]", path)
			//context.SchStream <- path
			go fileMoratorium(path, context.SchStream, moratoriumTime)
		} else if PathIsDirectory(path) {
			for _, ignoreDir := range ignoreDirs {
				if info.Name() == ignoreDir {
					Log.Debugf("Skipping directory without errors [%v]", path)
					return filepath.SkipDir
				}
			}
			// Directory case
			if err := watcher.Add(path); err != nil {
				Log.Criticalf("Couldn't add subdir %s watch [%v]", path, err)
				context.ReqQueue <- ThreadCannotContinue
				procErr := fmt.Errorf("Unable to add directory %s to watch [%v]", path, err)
				return procErr
			}
			Log.Noticef("Added subdirectory to watch [%v]", path)
		}
		return nil
	}

	// Add watch directories to watcher
	var nOrigDirs uint = 0
	if viper.IsSet("watcher.dir") {
		// n=1 case
		watchDir := viper.GetString("watcher.dir")
		if !PathIsDirectory(watchDir) {
			Log.Criticalf("Watch directory does not exist or is not a directory:\n\t%s", watchDir)
			context.ReqQueue <- ThreadCannotContinue
			return
		}
                if recProcErr := filepath.Walk(watchDir, processRecursiveDir); recProcErr != nil {
			Log.Criticalf("Error processing directory or file [%s]\n\t:%v", watchDir, recProcErr)
			context.ReqQueue <- ThreadCannotContinue
			return
		}
		watcher.Add(watchDir)
		Log.Noticef("Now watching <%s>", watchDir)
		nOrigDirs++
	}
	if viper.IsSet("watcher.dirs") {
		// n>1 case
		watchDirs := viper.GetStringSlice("watcher.dirs")
		for _, watchDir := range watchDirs {
			if !PathIsDirectory(watchDir) {
				Log.Criticalf("Watch directory does not exist or is not a directory:\n\t%s", watchDir)
				context.ReqQueue <- ThreadCannotContinue
				return
			}
			if recProcErr := filepath.Walk(watchDir, processRecursiveDir); recProcErr != nil {
				Log.Criticalf("Error processing directory or file [%s]\n\t:%v", watchDir, recProcErr)
				context.ReqQueue <- ThreadCannotContinue
				return
			}
			watcher.Add(watchDir)
			Log.Noticef("Now watching <%s>", watchDir)
			nOrigDirs++
		}
	}
	if nOrigDirs == 0 {
		// n=0 case
		Log.Critical("No watch directories were specified")
		context.ReqQueue <- ThreadCannotContinue
		return
	}

	Log.Info("Started successfully. Waiting for events...")

runLoop:
	for {
		select {

		case control, queueOk := <-context.CtrlQueue:
			if !queueOk {
				Log.Error("Control queue has closed unexpectedly")
				break runLoop
			}
			// Stop signal from CtrlQueue
			if control == StopExecution {
				Log.Info("Stopping on interrupt.")
				break runLoop
			}

		case newEvent, queueOk := <-watcher.Events:
			if !queueOk {
				Log.Error("Watcher event queue has closed unexpectedly")
				context.ReqQueue <- StopExecution
				break runLoop
			}

			if !shouldPayAttention(newEvent) {
				continue
			}

			// New subdirectory OR file is created
			fileName := newEvent.Name

			if recProcErr := filepath.Walk(fileName, processRecursiveDir); recProcErr != nil {
				Log.Criticalf("Error processing directory or file [%s]\n\t:%v", fileName, recProcErr)
				context.ReqQueue <- ThreadCannotContinue
				break runLoop
			}
			/*
				if PathIsRegularFile( fileName ) && shouldPayAttention( newEvent ) {
					// File case
					Log.Debug( "Submitting file [%v]", fileName )
					//context.SchStream <- fileName
					go fileMoratorium(fileName, context.SchStream, moratoriumTime)
				} else if PathIsDirectory( fileName ) && shouldPayAttention( newEvent ) {
					// Directory case
					if err := watcher.Add( fileName ); err != nil {
						Log.Critical( "Couldn't add subdir dir watch [%v]", err )
						context.ReqQueue <- ThreadCannotContinue
						break runLoop
					}
					Log.Notice( "Added subdirectory to watch [%v]", fileName )
				}
			*/
		case watchErr, queueOk := <-watcher.Errors:
			if !queueOk {
				Log.Error("Watcher error queue has closed unexpectedly")
				context.ReqQueue <- StopExecution
				break runLoop
			}

			// Error thrown on watcher
			if !isEintr(watchErr) {

				// Specific error detected by isEintr() is passable
				Log.Criticalf("fsnotify error on watch %v", watchErr)
				context.ReqQueue <- ThreadCannotContinue
				break runLoop
			}
		} // select
	} // for
} // Watcher
