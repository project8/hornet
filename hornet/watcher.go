/*
* watcher.go
*
* watcher is responsible for monitoring changes to the filesystem and sending
* those changes to workers.
 */
package hornet

import (
	"log"
	"strings"

	"github.com/spf13/viper"
	"golang.org/x/exp/inotify"
)

const (
	dirCreatedMask   = inotify.IN_ISDIR | inotify.IN_CREATE
	dirMovedToMask   = inotify.IN_ISDIR | inotify.IN_MOVED_TO
	//dirMovedFromMask = inotify.IN_ISDIR | inotify.IN_MOVED_FROM
	//dirDeletedMask   = inotify.IN_ISDIR | inotify.IN_DELETE
	ignoreMask = inotify.IN_IGNORED
)

// shouldAddWatch tests to see if this is a new directory or if this directory
// was moved to a place where it should be watched.
func shouldAddWatch(evt *inotify.Event) bool {
	newCreated := (evt.Mask & dirCreatedMask) == dirCreatedMask
	wasMovedTo := (evt.Mask & dirMovedToMask) == dirMovedToMask
	return newCreated || wasMovedTo
}

// shouldIgnore tests to see whether this is an event we should ignore.
// For instance, directory deletion events are caught by the filewatcher 
// with the IN_IGNORED bit set.
func shouldIgnore(evt *inotify.Event) bool {
	return (evt.Mask & inotify.IN_IGNORED) == ignoreMask
}

// shouldRemoveWatch tests to see if a directory is no longer of interest.
// NOTE: this functionality has been removed because testing has shown that 
//       deleted/moved directories are removed from the inotify watch automatically.
/*func shouldRemoveWatch(evt *inotify.Event) bool {
	wasDeleted := (evt.Mask & dirDeletedMask) == dirDeletedMask
	wasMovedFrom := (evt.Mask & dirMovedFromMask) == dirMovedFromMask
	return wasDeleted || wasMovedFrom
}*/

// isEintr is exactly what it sounds like.
func isEintr(e error) bool {
	return e != nil && strings.Contains(e.Error(), "interrupted system call")
}

// Inotify flags.  We only monitor for file close events, i.e. the data in the
// file is fixed.
const fileWatchFlags = inotify.IN_CLOSE_WRITE
const subdWatchFlags = inotify.IN_ONLYDIR | inotify.IN_CREATE | inotify.IN_MOVED_TO | inotify.IN_DELETE | inotify.IN_MOVED_FROM

// Watcher uses inotify to monitor changes to a specific path.
func Watcher(context OperatorContext) {
	// Decrement the waitgroup counter when done
	defer context.PoolCount.Done()
	defer log.Print("[watcher] finished.")

	watchDir := viper.GetString("watcher.dir")

	fileWatch, fileWatchErr := inotify.NewWatcher()
	if fileWatchErr != nil {
		log.Printf("[watcher] error creating file watcher! %v", fileWatchErr)
		context.ReqQueue <- ThreadCannotContinue
	} else {
		fileWatch.AddWatch(watchDir, fileWatchFlags)
	}
	defer fileWatch.Close()
	log.Printf("[watcher debug] file flags: %v", fileWatchFlags)
	log.Printf("[watcher debug] subd flags: %v", subdWatchFlags)

	subdWatch, subdWatchErr := inotify.NewWatcher()
	if subdWatchErr != nil {
		log.Printf("[watcher] error creating subdir watcher! %v", subdWatchErr)
		context.ReqQueue <- ThreadCannotContinue
	} else {
		subdWatch.AddWatch(watchDir, subdWatchFlags)
	}
	defer subdWatch.Close()

	log.Print("[watcher] started successfully.  waiting for events...")

runLoop:
	for {
		select {
		// First check for any control messages.
		case control := <-context.CtrlQueue:
			if control == StopExecution {
				log.Print("[watcher] stopping on interrupt.")
				break runLoop
			}

		// if a new file is available in our watched directories, check to see
		// if we're supposed to do something - and if so, send it along.
		case fileCloseEvt := <-fileWatch.Event:
			if shouldIgnore(fileCloseEvt) {
				//log.Printf("[watcher file event (ignoring)] %s", fileCloseEvt.String())
				continue runLoop
			}
			//log.Printf("[watcher file event] %s", fileCloseEvt.String())
			context.SchStream <- fileCloseEvt.Name

		// directories are a little more complicated.  if it's a new directory,
		// watch it.  if it's a directory getting moved-from, delete the watch.
		// if it's a directory getting deleted, delete the watch.  if it's a
		// directory getting moved-to, watch it.
		case newSubDirEvt := <-subdWatch.Event:
			//log.Printf("[watcher dir event] %s", newSubDirEvt.String())
			dirname := newSubDirEvt.Name
			if shouldAddWatch(newSubDirEvt) {
				if err := fileWatch.AddWatch(dirname, fileWatchFlags); err != nil {
					log.Printf("[watcher] couldn't add subdir file watch [%v]", err)
					context.CtrlQueue <- ThreadCannotContinue
					break runLoop
				} else {
					log.Printf("[watcher] added subdirectory to file watch [%v]", dirname)
				}
				if err := subdWatch.AddWatch(dirname, subdWatchFlags); err != nil {
					log.Printf("[watcher] couldn't add subdir dir watch [%v]", err)
					context.CtrlQueue <- ThreadCannotContinue
					break runLoop
				} else {
					log.Printf("[watcher] added subdirectory to dir watch [%v]", dirname)
				}
			} /*else if shouldRemoveWatch(newSubDirEvt) {
				log.Printf("[watcher] removing watch on %s...", dirname)
				if err := fileWatch.RemoveWatch(dirname); err != nil {
					log.Printf("[watcher] can't remove file watch on %s [%v]", dirname, err)
				} else {
					log.Printf("[watcher] removed file watch on %s", dirname)
				}
				if err := subdWatch.RemoveWatch(dirname); err != nil {
					log.Printf("[watcher] can't remove dir watch on %s [%v]", dirname, err)
				} else {
					log.Printf("[watcher] removed dir watch on %s", dirname)
				}
			}*/

			//	if either of the filesystem watchers gets an error, die
			//	as gracefully as possible.
		case fileWatchErr = <-fileWatch.Error:
			if isEintr(fileWatchErr) == false {
				log.Printf("[watcher] inotify error on file watch %v",
					fileWatchErr)
				context.ReqQueue <- ThreadCannotContinue
				break runLoop
			}

		case subdWatchErr = <-subdWatch.Error:
			if isEintr(fileWatchErr) == false {
				log.Printf("[watcher] inotify error on directory watch %v",
					subdWatchErr)
				context.ReqQueue <- ThreadCannotContinue
				break runLoop
			}

		}
	}
}

