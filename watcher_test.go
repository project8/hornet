// testing for the filesystem watcher.
package main

import (
	. "gopkg.in/check.v1"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// suite type for the watcher fixture
type HornetWatcherSuite struct {
	cxt Context
	cfg Config
}

// create all the necessary directories and what have you, and start the
// mover thread.
func (s *HornetWatcherSuite) SetUpSuite(c *C) {
	// waitgroup for just the mover
	var wg sync.WaitGroup

	// temporary input and output directories
	tempInDir, tidErr := ioutil.TempDir("", "hornet_test_in_dir")
	tempOutDir, todErr := ioutil.TempDir("", "hornet_test_out_dir")
	if (tidErr != nil) || (todErr != nil) {
		panic("creation of temporary directories failed!\n")
	}

	s.cfg = Config{
		WatchDirPath: tempInDir,
		DestDirPath:  tempOutDir,
	}

	s.cxt = Context{
		Control:            make(chan ControlMessage),
		NewFileStream:      make(chan string, 3),
		Pool:               &wg,
		FinishedFileStream: make(chan string, 3),
	}

	s.cxt.Pool.Add(1)
	go Watcher(s.cxt, s.cfg)
}

// tests
var _ = Suite(&HornetWatcherSuite{})

func TestCheckWatcher(t *testing.T) { TestingT(t) }

func (s *HornetWatcherSuite) TestWatcherNotifyOnSubdir(c *C) {
	// create a subdirectory in the watched base directory.  create a fake
	// file there that should trigger the watcher to send the filename into
	// the NewFileStream channel.  Check that we receive the file on the other
	// end.  If we don't, that's failure.
	tmpSubDir, tmpErr := ioutil.TempDir(s.cfg.WatchDirPath, "watch_subdir_test")
	if tmpErr != nil {
		c.Logf("couldn't create subdir for testing! [%v]", tmpErr)
		c.Fail()
	}

	time.Sleep(10 * time.Millisecond)

	tmpFileName := strings.Join([]string{tmpSubDir, "watch_file.MAT"}, "/")
	tmpFile, tmpFileErr := os.Create(tmpFileName)
	if tmpFileErr != nil {
		c.Logf("couldn't create temporary file for testing! [%v]", tmpFileErr)
		c.Fail()
	}
	tmpFile.WriteString("hornet watcher test file")
	tmpFile.Close()

	time.Sleep(10 * time.Millisecond)

	select {
	case fname := <-s.cxt.NewFileStream:
		if fname != tmpFileName {
			c.Fail()
		}
	default:
		c.Log("no message available from watcher!")
		c.Fail()
	}
}
