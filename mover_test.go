/*
* mover_test.go
* 
* a test suite for the hornet mover
*/
package main

import (
	"testing"
	"sync"
	"syscall"
	"os"
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
	cfg := Config{DestDirPath: "./testing"}
	cxt := Context{Pool: &wg, 
		OutputFileStream: fileStream,
		Control: control,}
	go Mover(cxt, cfg)
	
	// TODO: should use actual temporary facilities here, but I'm not
	// paying 30$ to get on the in-flight internet to check the API.
	// for now, this is going to fail.  the outline is about right 
	// though.
	tempName := "hornet_mover_test"
	if _, err := syscall.Creat(tempName, 0777); err != nil {
		fileStream <- tempName
	} else {
		t.Log("couldn't create temp file for test!")
		t.Fail()
	}

	if _, mvErr := os.Stat("./testing/hornet_mover_test"); mvErr != nil {
		t.Log("couldn't stat file after moving!")
		t.Fail()
	}
}
