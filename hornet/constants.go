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

// Project 8 Wire Protocol Standards
type MsgType uint64
const (
	Reply   MsgType = 2
	Request MsgType = 3
	Alert   MsgType = 4
	Info    MsgType = 5
)

type MsgOp uint64
const (
	Set     MsgOp = 0
	Get     MsgOp = 1
	Config  MsgOp = 6
	Send    MsgOp = 7
	Run     MsgOp = 8
	Command MsgOp = 9
)
