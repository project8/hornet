// Utility functions for the hornet package go here.
package hornet

import (
	//"errors"
	//"fmt"
	"os"
	"path/filepath"
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
// has as part of it the base path.
// It behaves in one of two ways:
//  1. If the filename starts with the base path, it renames the file by replacing
//     the base path with the target path, and maintaining any subdirectory structure
//     that the filename may have:
//        e.g. RenamePathRelativeTo("/abc/def/foo.bar", "/abc", "/ghi") ->
//             "/ghi/def/foo.bar".
//  2. If base is not a prefix of filename, the filename, minus its diretory path,
//     is postfixed onto the target path; there will be no subdirectory structure.
func RenamePathRelativeTo(filename, base, dest string) (s string, e error) {
	if strings.HasPrefix(filename, base) {
		subpath, relErr := filepath.Rel(base, filename)
		if relErr != nil {
			e = relErr
			return
		}
		s = filepath.Join(dest, subpath)
		//fmt.Printf("RPRT opt 1: %s --> %s\n", filename, s)
		//e = errors.New("filename does not contain base as a prefix")
		return
	} else {
		_, file := filepath.Split(filename)
		s = filepath.Join(dest, file)
		//fmt.Printf("RPRT opt 2: %s --> %s\n", filename, s)
		return
	}
}
