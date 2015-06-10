/*
* constants.go
*
* the set of constants that might be used throughout hornet
*
*/

package hornet

// Global config
var (
	MaxThreads int = 25
)

// A ControlMessage is sent between the main thread and the worker threads
// to indicate system events (such as termination) that must be handled.
type ControlMessage uint

const (
	// StopExecution asks the worker threads to finish what they are doing
	// and return gracefully.
	StopExecution = 0

	// ThreadCannotContinue signals that the sending thread cannot continue
	// executing due to an error, and hornet should shut down.
	ThreadCannotContinue = 1
)

// Time format
const TimeFormat = "2006-01-02T22:04:05Z"


// Project 8 Wire Protocol Standards
type MsgCodeT uint64

const (
	MTReply   MsgCodeT = 2
	MTRequest MsgCodeT = 3
	MTAlert   MsgCodeT = 4
	MTInfo    MsgCodeT = 5
)

const (
	MOSet     MsgCodeT = 0
	MOGet     MsgCodeT = 1
	MOConfig  MsgCodeT = 6
	MOSend    MsgCodeT = 7
	MORun     MsgCodeT = 8
	MOCommand MsgCodeT = 9
)

const (
	RCSuccess MsgCodeT = 0
)

