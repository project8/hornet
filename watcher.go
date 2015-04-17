/*
* watcher.go
*
* watcher is responsible for monitoring changes to the filesystem and sending
* those changes to workers.
 */
package main

import (
	"golang.org/x/exp/inotify"
	"log"
	"strings"
)

// isTargetFile returns true if the file should be passed along to a worker
// thread for processing.
func isTargetFile(s string) bool {
	return strings.HasSuffix(s, ".MAT")
}

// Inotify flags.  We only monitor for file close events, i.e. the data in the
// file is fixed.
const fileWatchFlags = inotify.IN_CLOSE_WRITE
const subdWatchFlags = inotify.IN_ONLYDIR | inotify.IN_CREATE

// Watcher uses inotify to monitor changes to a specific path.
func Watcher(context Context, config Config) {
	// Decrement the waitgroup counter when done
	defer context.Pool.Done()

	fileWatch, fileWatchErr := inotify.NewWatcher()
	if fileWatchErr != nil {
		log.Printf("(watcher) error creating file watcher! %v", fileWatchErr)
		context.Control <- ThreadCannotContinue
	} else {
		fileWatch.AddWatch(config.WatchDirPath, fileWatchFlags)
	}
	defer fileWatch.Close()

	subdWatch, subdWatchErr := inotify.NewWatcher()
	if subdWatchErr != nil {
		log.Printf("(watcher) error creating subdir watcher! %v", subdWatchErr)
		context.Control <- ThreadCannotContinue
	} else {
		subdWatch.AddWatch(config.WatchDirPath, subdWatchFlags)
	}
	defer subdWatch.Close()

	log.Print("watcher started successfully.  waiting for events...")

runLoop:
	for {
		select {
		// First check for any control messages.
		case control := <-context.Control:
			if control == StopExecution {
				log.Print("watcher stopping on interrupt.")
				break runLoop
			}

		// if a new file is available in our watched directories, check to see
		// if we're supposed to do something - and if so, send it along.
		case fileCloseEvt := <-fileWatch.Event:
			fname := fileCloseEvt.Name
			if isTargetFile(fname) {
				context.NewFileStream <- fname
			}

		// in the event a new subdirectory has been created in the base watch
		// directory, add that directory to the list of watched locations that
		// the file watcher is looking at.
		case newSubDir := <-subdWatch.Event:
			dirname := newSubDir.Name
			if err := fileWatch.AddWatch(dirname, fileWatchFlags); err != nil {
				log.Printf("couldn't add subdir watch! [%v]", err)
				context.Control <- ThreadCannotContinue
				break runLoop
			} else {
				log.Printf("(subdir watcher) added subdirectory to watch [%v]",
					dirname)
			}

			//	if either of the filesystem watchers gets an error, die
			//	as gracefully as possible.
		case fileWatchErr = <-fileWatch.Error:
			log.Printf("(file watcher) inotify error! %v", fileWatchErr)
			context.Control <- ThreadCannotContinue
			break runLoop

		case subdWatchErr = <-subdWatch.Error:
			log.Printf("(subdir watcher) inotify error! %v", subdWatchErr)
			context.Control <- ThreadCannotContinue
			break runLoop
		}
	}
	close(context.NewFileStream)
}
