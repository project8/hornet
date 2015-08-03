package hornet

import(
	"strings"
	"github.com/spf13/viper"
	"gopkg.in/fsnotify.v1"
)

func shouldPayAttention( evt fsnotify.Event ) bool {
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
	watcher, watcherErr := fsnotify.NewWatcher()
	if watcherErr != nil {
		Log.Critical( "Could not create the watcher! %v", watcherErr )
		context.ReqQueue <- ThreadCannotContinue
		return
	}
	defer watcher.Close()

	// Add watch directories to watcher
	var nOrigDirs uint = 0
	if viper.IsSet( "watcher.dir" ) {
		// n=1 case
		watchDir := viper.GetString( "watcher.dir" )
		if !PathIsDirectory( watchDir ) {
			Log.Critical( "Watch directory does not exist or is not a directory:\n\t%s", watchDir )
			context.ReqQueue <- ThreadCannotContinue
			return
		}
		watcher.Add( watchDir )
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
			watcher.Add( watchDir )
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

		case newEvent := <-watcher.Events:
			// New subdirectory OR file is created
			fileName := newEvent.Name

			if PathIsRegularFile( fileName ) && shouldPayAttention( newEvent ) {
				// File case
				Log.Debug( "Submitting file [%v]", fileName )
				context.SchStream <- fileName
			} else if PathIsDirectory( fileName ) && shouldPayAttention( newEvent ) {
				// Directory case
				if err := watcher.Add( fileName ); err != nil {
					Log.Critical( "Couldn't add subdir dir watch [%v]", err )
					context.ReqQueue <- ThreadCannotContinue
					break runLoop
				}
				Log.Notice( "Added subdirectory to watch [%v]", fileName )
			}

		case watchErr := <-watcher.Errors:
			// Error thrown on watcher
			if !isEintr( watchErr ) {

				// Specific error detected by isEintr() is passable
				Log.Critical( "fsnotify error on watch %v", watchErr )
				context.ReqQueue <- ThreadCannotContinue
				break runLoop
			}
		} 	// select
	} 		// for
}			// Watcher