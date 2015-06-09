/*
* fileinfo.go
*
* The FileInfo struct holds the header information about a file being processed by Hornet.
 */

package hornet

import (
	"text/template"
)

type Job struct {
	CommandTemplate  *template.Template
	CommandName      string
	CommandArgs      [] string
}

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
	JobQueue        chan Job
	FinishedJobs    []Job
}

// Base paths are special locations on top of which a file system exists
// These are defined in the classifier config; if a watcher is in use, its path is added to this
var BasePaths []string

