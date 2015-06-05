/*
* fileinfo.go
*
* The FileInfo struct holds the header information about a file being processed by Hornet.
 */

package hornet

import (
	//"fmt"
	//"path/filepath"
	"strings"
)

// File information header
type FileInfo struct {
	Filename        string
	FileType        string
	FileHash        string
	SubPath         string
	HotPath         string
	WarmPath        string
	ColdPath        string
	FileHotPath     string
	FileWarmPath    string
    FileColdPath    string
	SecondaryFiles  []string
	DoNearline      bool
	NearlineCmdName string
	NearlineCmdArgs []string
}

// AddSecondaryFile appends another filename to the list of secondary files
func (fileInfoPtr *FileInfo) AddSecondaryFile(file string) {
	(*fileInfoPtr).SecondaryFiles = append((*fileInfoPtr).SecondaryFiles, file)
	return
}

// SetNearlineCmd splits the command name from its arguments, and sets the name in the FileInfo struct
func (fileInfoPtr *FileInfo) SetNearlineCmd(nearlineCmd string) {
	fileInfo := *fileInfoPtr
	commandParts := strings.Fields(nearlineCmd)
	fileInfo.NearlineCmdName = commandParts[0]
	fileInfo.NearlineCmdArgs = commandParts[1:len(commandParts)]
	*fileInfoPtr = fileInfo
	return
}


// Base paths are special locations on top of which a file system exists
// These are defined in the classifier config; if a watcher is in use, its path is added to this
var BasePaths []string

