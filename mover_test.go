/*
* mover_test.go
* 
* a test suite for the hornet mover
*/
package main

import (
	"testing"
	"sync"
	"os"
	"io/ioutil"
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

// Test that a temporary file gets correctly moved by the mover thread
// when its name is sent through the output channel.
func TestMover(t *testing.T) {
	var wg sync.WaitGroup
	fileStream := make(chan string)
	control := make(chan ControlMessage)
	
	// TODO: should use actual temporary facilities here, but I'm not
	// paying 30$ to get on the in-flight internet to check the API.
	// for now, this is going to fail.  the outline is about right 
	// though.
	tempInDir, tidErr := ioutil.TempDir("","hornet_test_in_dir")
	tempOutDir, todErr := ioutil.TempDir("","hornet_test_out_dir")
	if (tidErr != nil) || (todErr != nil) {
		t.Logf("creation of temporary directories failed!\n")
		t.Fail()
	}

	// create a temporary file to work with
	tempFile, fileErr := ioutil.TempFile(tempInDir, "hornet_mover_test")
	if fileErr != nil {
		t.Logf("couldn't create temporary file!\n")
		t.Fail()
	}
	t.Logf("created file at %s\n", tempFile.Name())

	// write a tiny amount of data to the file
	_, wErr := tempFile.WriteString("this is a test file created by hornet.")
	tempFileName := tempFile.Name()
	if wErr != nil {
		t.Logf("couldn't write to temporary file!\n")
		t.Fail()
	}
	tempFile.Close()

	// alright, now we need to know both where we expect to find the file
	// to begin with and where we expect it to wind up...
	tempOutPath := MovedFilePath(tempFileName, tempOutDir)

	cfg := Config{DestDirPath: tempOutDir}
	cxt := Context{Pool: &wg, 
		OutputFileStream: fileStream,
		Control: control,}
	go Mover(cxt, cfg)

	// now send the file to the mover.
	fileStream <- tempFileName

	// now look to see that the mover actually moved it.
	if _, movedErr := os.Stat(tempOutPath); movedErr != nil {
		t.Logf("file does not appear to have moved!")
		t.Fail()
	}


}
