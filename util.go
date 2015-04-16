// Utility functions for the hornet package go here.
package main

import (
	"os"
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
