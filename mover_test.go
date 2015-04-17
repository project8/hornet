/*
* mover_test.go
*
* a test suite for the hornet mover
 */
package main

import (
	. "gopkg.in/check.v1"
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"time"
)

func TestFilenameRenamingAbsolute(t *testing.T) {
	in := "/abc/def/ghi.MAT"
	tgt := "/jkl/mno/ghi.MAT"
	if out := MovedFilePath(in, "/jkl/mno"); out != tgt {
		t.Logf("rename failed: %s != %s.\n", out, tgt)
		t.Fail()
	}
}

func TestFilenameRenamingRelative(t *testing.T) {
	in := "ghi.MAT"
	tgt := "/jkl/mno/ghi.MAT"
	if out := MovedFilePath(in, "/jkl/mno"); out != tgt {
		t.Logf("rename failed: %s != %s.\n", out, tgt)
		t.Fail()
	}
}

func TestGoCheck(t *testing.T) { TestingT(t) }

type HornetSuite struct {
	cfg Config
	cxt Context
}

var _ = Suite(&HornetSuite{})

// create all the necessary directories and what have you, and start the
// mover thread.
func (s *HornetSuite) SetUpSuite(c *C) {
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
	go Mover(s.cxt, s.cfg)
}

// stop the mover thread and delete all temporary directories and shit.
func (s *HornetSuite) TearDownSuite(c *C) {
	s.cxt.Control <- StopExecution
	s.cxt.Pool.Wait()

	//os.RemoveAll(s.cfg.DestDirPath)
	//os.RemoveAll(s.cfg.WatchDirPath)
}

// test that simply moving a file works correctly - the file moves and is no
// no longer in its original place.
func (s *HornetSuite) TestMoveWorks(c *C) {

	tempFile, fileErr := ioutil.TempFile(s.cfg.WatchDirPath, "hornet_mover_test")
	if fileErr != nil {
		c.Logf("couldn't create temporary file!\n")
		c.Fail()
	}
	c.Logf("created file at %s\n", tempFile.Name())

	_, wErr := tempFile.WriteString("this is a test file created by hornet.")
	tempFileName := tempFile.Name()
	if wErr != nil {
		c.Logf("couldn't write to temporary file!\n")
		c.Fail()
	}
	tempFile.Close()

	s.cxt.FinishedFileStream <- tempFileName
	tempExpectedOut := MovedFilePath(tempFileName, s.cfg.DestDirPath)

	time.Sleep(100 * time.Millisecond)

	if _, srcErr := os.Stat(tempFileName); srcErr == nil {
		c.Logf("temp file still in original location!")
		c.Fail()
	}

	if _, movedErr := os.Stat(tempExpectedOut); movedErr != nil {
		c.Logf("file does not appear to have moved!")
		c.Fail()
	}
}
