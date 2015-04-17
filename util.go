// Utility functions for the hornet package go here.
package main

import (
	"errors"
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

// RenamePathRelativeTo takes a base path, a target path, and a filename which
// has as part of it the base path.  It then renames the file, respecting its
// base path and maintaining any subdirectory structure that the filename may
// have: e.g. RenamePathRelativeTo("/abc/def/foo.bar", "/abc", "/ghi") ->
// "/ghi/def/foo.bar".  If base is not a prefix of filename, the function call
// is considered an error.
func RenamePathRelativeTo(filename, base, dest string) (s string, e error) {
	if strings.HasPrefix(filename, base) == false {
		e = errors.New("filename does not contain base as a prefix")
		return
	}

	sep := "/"
	relPath := strings.TrimPrefix(filename, base)
	if strings.HasPrefix(relPath, sep) {
		sep = ""
	}
	s = strings.Join([]string{dest, relPath}, sep)
	return
}
