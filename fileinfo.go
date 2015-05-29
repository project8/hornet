/*
* fileinfo.go
*
* The FileInfo struct holds the header information about a file being processed by Hornet.
 */

package main

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
	HotPath         string
	WarmPath        string
	ColdPath        string
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
