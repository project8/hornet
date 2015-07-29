package hornet

import(
	"strings"
	"github.com/spf13/viper"
	"gopkg.in/fsnotify.v1"
)
/*
const(
	dirCreatedMask = syscall.IN_ISDIR | syscall.IN_CREATE
	dirMovedToMask = syscall.IN_ISDIR | syscall.IN_MOVED_TO
	ignoreMask = syscall.IN_IGNORED
)
*/
func shouldAddWatch( evt fsnotify.Event ) bool {
	newCreated := evt.Op == fsnotify.Create
	wasMovedTo := evt.Op == fsnotify.Rename
	return newCreated || wasMovedTo
}

//func shouldIgnore( evt *fsnotify.Event ) bool {
//	return (evt.Mask & syscall.IN_IGNORED) == ignoreMask
//}

func isEintr( e error ) bool {
	return e != nil && strings.Contains( e.Error(), "interrupted system call" )
}

//const fileWatchFlags = syscall.IN_CLOSE_WRITE
//const subdWatchFlags = syscall.IN_ONLYDIR | syscall.IN_CREATE | syscall.IN_MOVED_TO | syscall.IN_DELETE | syscall.IN_MOVED_FROM

func Watcher( context OperatorContext ) {
	defer context.PoolCount.Done()
	defer Log.Info( "Watcher is finished." )

	fileWatch, fileWatchErr := fsnotify.NewWatcher()
	if fileWatchErr != nil {
		Log.Critical( "Could not create the file watcher! %v", fileWatchErr )
		context.ReqQueue <- ThreadCannotContinue
		return
	}
	defer fileWatch.Close()

	subdWatch, subdWatchErr := fsnotify.NewWatcher()
	if subdWatchErr != nil {
		Log.Critical( "Could not create the subdirectory watcher! %v", subdWatchErr )
		context.ReqQueue <- ThreadCannotContinue
		return
	}
	defer subdWatch.Close()

	var nOrigDirs uint = 0
	if viper.IsSet( "watcher.dir" ) {
		watchDir := viper.GetString( "watcher.dir" )
		if !PathIsDirectory( watchDir ) {
			Log.Critical( "Watch directory does not exist or is not a directory:\n\t%s", watchDir )
			context.ReqQueue <- ThreadCannotContinue
			return
		}
		fileWatch.Add( watchDir )
		subdWatch.Add( watchDir )
		Log.Notice( "Now watching <%s>", watchDir )
		nOrigDirs++
	}
	if viper.IsSet( "watcher.dirs" ) {
		watchDirs := viper.GetStringSlice( "watcher.dirs" )
		for _, watchDir := range watchDirs {
			if !PathIsDirectory( watchDir ) {
				Log.Critical(" Watch directory does not exist or is not a directory:\n\t%s", watchDir )
				context.ReqQueue <- ThreadCannotContinue
				return
			}
			fileWatch.Add( watchDir )
			subdWatch.Add( watchDir )
			Log.Notice( "Now watching <%s>", watchDir )
			nOrigDirs++
		}
	}
	if nOrigDirs == 0 {
		Log.Critical( "No watch directories were specified" )
		context.ReqQueue <- ThreadCannotContinue
		return
	}

	Log.Info( "Started successfully. Waiting for events..." )

runLoop:
	for {
		select {

		case control := <-context.CtrlQueue:
			if control == StopExecution {
				Log.Info( "Stopping on interrupt." )
				break runLoop
			}

		case fileCloseEvt := <-fileWatch.Events:
			//if shouldIgnore( fileCloseEvt ) {
			//	continue runLoop
			//}
			context.SchStream <- fileCloseEvt.Name
		
		case newSubDirEvt := <-subdWatch.Events:
			dirName := newSubDirEvt.Name
			if !PathIsDirectory( dirName ) {
				continue runLoop
			}
			if shouldAddWatch( newSubDirEvt ) {
				if err := fileWatch.Add( dirName ); err != nil {
					Log.Critical( "Couldn't add subdir file watch [%v]", err )
					context.ReqQueue <- ThreadCannotContinue
					break runLoop
				}
				if err := subdWatch.Add( dirName ); err != nil {
					Log.Critical( "Couldn't add subdir dir watch [%v]", err )
					context.ReqQueue <- ThreadCannotContinue
					break runLoop
				}
				Log.Notice( "Added subdirectory to file & subdirectory watches [%v]", dirName )
			}

		case fileWatchErr = <-fileWatch.Errors:
			if !isEintr( fileWatchErr ) {
				Log.Critical( "fsnotify error on file watch %v", fileWatchErr )
				context.ReqQueue <- ThreadCannotContinue
				break runLoop
			}

		case subdWatchErr = <-subdWatch.Errors:
			if !isEintr( fileWatchErr ) {
				Log.Critical( "fsnotify error on directory watch %v", subdWatchErr )
				context.ReqQueue <- ThreadCannotContinue
				break runLoop
			}
		} 	// select
	} 		// for
}			// method