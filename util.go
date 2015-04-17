// Utility functions for the hornet package go here.
package main

import (
	"os"
	"strings"
)

// PathIsDirectory returns true if the string argument is a path to a
// directory on the filesystem.  If the directory does not exist or cannot
// be stat-ed, it also returns false - in other words, this function does
// not distinguish between a regular file and a non-existent directory with
// respect to its return value.
func PathIsDirectory(path string) bool {
	if info, err := os.Stat(path); err == nil {
		return info.IsDir()
	}

	return false

}

// MovedFilePath takes a path to a file as its argument, and the directory to
// which that file is to be moved.  It returns the new path as a string e.g.
// MovedFilePath("/abc/def.xxx","/ghi") -> "/ghi/def.xxx"
func MovedFilePath(orig, newdir string) (newpath string) {
	var namepos int
	var sep string
	if namepos = strings.LastIndex(orig, "/"); namepos == -1 {
		namepos = 0
		if strings.HasSuffix(newdir, "/") == false {
			sep = "/"
		}
	}
	newpath = strings.Join([]string{newdir, orig[namepos:]}, sep)

	return
}
