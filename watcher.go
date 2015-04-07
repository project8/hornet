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
)

// Inotify flags.  We only monitor for file close events, i.e. the data in the
// file is fixed.
const watchFlags = inotify.IN_CLOSE_WRITE

// Watcher uses inotify to monitor changes to a specific path.
func Watcher(context Context, config Config) {
	// Decrement the waitgroup counter when done
	defer context.Pool.Done()

	inot, inotErr := inotify.NewWatcher()
	if inotErr != nil {
		log.Fatal("error creating watcher!", inotErr)
	} else {
		inot.AddWatch(config.WatchDirPath, watchFlags)
	}
	defer inot.Close()

	log.Print("watcher started successfully.  waiting for events...")

	var control ControlMessage
	for {
		select {
		// First check for any control messages.
		case control = <-context.Control:
			if control == StopExecution {
				log.Print("stop requested.  watcher stopping...")
				break
			}
		case inotEvt := <-inot.Event:
			// FIXME: strings.join may be inefficient, probably should
			// use bytes.Buffer instead.  we're also building a
			// slice every time we get a new file, which is silly.
			fname := inotEvt.Name
			context.FilePipeline <- fname
		case inotErr = <-inot.Error:
			log.Printf("inotify error! %v", inotErr)
		}

		// TODO: This is a little weird, surely we can do this better.
		if control == StopExecution {
			break
		}
	}
}
