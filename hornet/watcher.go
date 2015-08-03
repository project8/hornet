package hornet

import(
	"strings"
	"github.com/spf13/viper"
	"gopkg.in/fsnotify.v1"
)

func shouldAddWatch( evt fsnotify.Event ) bool {
	// Returns true if the event was triggered by file creation or rename (i.e. move)
	newCreated := evt.Op == fsnotify.Create
	wasMovedTo := evt.Op == fsnotify.Rename
	return newCreated || wasMovedTo
}

func isEintr( e error ) bool {
	// Detects particular system interrupt error
	return e != nil && strings.Contains( e.Error(), "interrupted system call" )
}

func Watcher( context OperatorContext ) {
	defer context.PoolCount.Done()
	defer Log.Info( "Watcher is finished." )

	// New subdirectory watcher
	subdWatch, subdWatchErr := fsnotify.NewWatcher()
	if subdWatchErr != nil {
		Log.Critical( "Could not create the subdirectory watcher! %v", subdWatchErr )
		context.ReqQueue <- ThreadCannotContinue
		return
	}
	defer subdWatch.Close()

	// Add watch directories to fileWatch and subdWatch
	var nOrigDirs uint = 0
	if viper.IsSet( "watcher.dir" ) {

		// n=1 case
		watchDir := viper.GetString( "watcher.dir" )
		if !PathIsDirectory( watchDir ) {
			Log.Critical( "Watch directory does not exist or is not a directory:\n\t%s", watchDir )
			context.ReqQueue <- ThreadCannotContinue
			return
		}
		subdWatch.Add( watchDir )
		Log.Notice( "Now watching <%s>", watchDir )
		nOrigDirs++
	}
	if viper.IsSet( "watcher.dirs" ) {

		// n>1 case
		watchDirs := viper.GetStringSlice( "watcher.dirs" )
		for _, watchDir := range watchDirs {
			if !PathIsDirectory( watchDir ) {
				Log.Critical( "Watch directory does not exist or is not a directory:\n\t%s", watchDir )
				context.ReqQueue <- ThreadCannotContinue
				return
			}
			subdWatch.Add( watchDir )
			Log.Notice( "Now watching <%s>", watchDir )
			nOrigDirs++
		}
	}
	if nOrigDirs == 0 {
		// n=0 case
		Log.Critical( "No watch directories were specified" )
		context.ReqQueue <- ThreadCannotContinue
		return
	}

	Log.Info( "Started successfully. Waiting for events..." )

runLoop:
	for {
		select {

		case control := <-context.CtrlQueue:
			// Stop signal from CtrlQueue
			if control == StopExecution {
				Log.Info( "Stopping on interrupt." )
				break runLoop
			}

		case newSubDirEvt := <-subdWatch.Events:
			// New subdirectory OR file is created
			dirName := newSubDirEvt.Name

			if !PathIsDirectory( dirName ) {
				// File case
				context.SchStream <- dirName
			} else if shouldAddWatch( newSubDirEvt ) {
				// Directory case
				if err := subdWatch.Add( dirName ); err != nil {
					Log.Critical( "Couldn't add subdir dir watch [%v]", err )
					context.ReqQueue <- ThreadCannotContinue
					break runLoop
				}
				Log.Notice( "Added subdirectory to file & subdirectory watches [%v]", dirName )
			}

		case subdWatchErr = <-subdWatch.Errors:
			// Error thrown on subdWatch
			if !isEintr( subdWatchErr ) {

				// Specific error detected by isEintr() is passable
				Log.Critical( "fsnotify error on directory watch %v", subdWatchErr )
				context.ReqQueue <- ThreadCannotContinue
				break runLoop
			}
		} 	// select
	} 		// for
}			// Watcher