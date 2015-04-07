/*
* hornet - a tool for nearline processing of Project 8 triggered data
*
*  https://github.com/kofron/hornet
*
* hornet uses inotify to start the processing of triggered data from the
* tektronix RSA.  it tries to stay out of the way by doing one job and
* doing it well, so that it is compatible with other workload management
* tools.
*
* all you need to tell it is the size of its thread pool for incoming files,
* the configuration file you want to run, and the path to Katydid.  it will
* handle the rest.
*
* for the most up to date usage information, do
*
*   hornet -h
*
 */
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Global config
var (
	MaxPoolSize uint = 25
)

// A ControlMessage is sent between the main thread and the worker threads
// to indicate system events (such as termination) that must be handled.
type ControlMessage uint

const (
	// StopExecution asks the worker threads to finish what they are doing
	// and return gracefully.
	StopExecution = 0

	// In the event that a thread cannot continue what it's doing due to 
	// an error, the main thread should shut hornet down.
	ThreadCannotContinue = 1
)

// Config represents the user-specified configuration data of hornet
type Config struct {
	PoolSize        uint
	WatchDirPath    string
	KatydidPath     string
	KatydidConfPath string
}

// Validate checks the sanity of a Config instance
//   1) the number of threads is sane
//   2) the provided path to katydid actually points at an executable
//   3) the provided config file is parsable json
//   4) the provided watch directory is indeed a directory
func (c Config) Validate() (e error) {
	if c.PoolSize > MaxPoolSize {
		e = fmt.Errorf("Size of thread pool cannot exceed %d", MaxPoolSize)
	}

	if execInfo, execErr := os.Stat(c.KatydidPath); execErr == nil {
		if (execInfo.Mode() & 0111) == 0 {
			e = fmt.Errorf("Katydid does not appear to be executable")
		}
	} else {
		e = fmt.Errorf("error when examining Katydid executable: %v",
			execErr)
	}

	if confBytes, confErr := ioutil.ReadFile(c.KatydidConfPath); confErr == nil {
		v := make(map[string]interface{})
		confDecoder := json.NewDecoder(bytes.NewReader(confBytes))
		if decodeErr := confDecoder.Decode(&v); decodeErr != nil {
			e = fmt.Errorf("Katydid config appears unparseable... %v",
				decodeErr)
		}
	} else {
		e = fmt.Errorf("error when opening Katydid config: %v",
			confErr)
	}

	if wdInfo, wdErr := os.Stat(c.WatchDirPath); wdErr == nil {
		if wdInfo.IsDir() == false {
			e = fmt.Errorf("watch directory must be a directory")
		}
	} else {
		e = fmt.Errorf("Problem opening watch directory: %v",
			wdErr)
	}

	return
}

// Context is the state of hornet
type Context struct {
	Pool         *sync.WaitGroup
	FilePipeline chan string
	Control      chan ControlMessage
}

func main() {
	// user needs help
	var needHelp bool

	// default configuration parameters.  these may be changed by
	// command line options.
	conf := Config{}

	// set up flag to point at conf, parse arguments and then verify
	flag.BoolVar(&needHelp,
		"help",
		false,
		"display this dialog")
	flag.UintVar(&conf.PoolSize,
		"pool-size",
		20,
		"size of worker pool")
	flag.StringVar(&conf.KatydidPath,
		"katydid-path",
		"REQUIRED",
		"full path to Katydid executable")
	flag.StringVar(&conf.KatydidConfPath,
		"katydid-conf",
		"REQUIRED",
		"full path to Katydid config file to use when processing")
	flag.StringVar(&conf.WatchDirPath,
		"watch-dir",
		"REQUIRED",
		"directory to watch for new data files")
	flag.Parse()

	if needHelp {
		flag.Usage()
		os.Exit(1)
	} else {
		if configErr := conf.Validate(); configErr != nil {
			flag.Usage()
			log.Fatal("(FATAL) ", configErr)
		}
	}

	// if we've made it this far, it's time to get down to business.
	// we'll start an inotify Notify request for the working directory.
	// then we have a single channel that we use to communicate with the
	// worker pool.  as we get file closed events, we check to see if we
	// already know about the file in question.  if not, and it is a
	// data file, we'll enqueue it to get chewed on by the next available
	// worker thread.
	// we use IN_CLOSE_WRITE here, we only care about file close events
	var pool sync.WaitGroup
	ctx := Context{
		FilePipeline: make(chan string, conf.PoolSize*3),
		Pool:         &pool,
		Control:      make(chan ControlMessage),
	}

	for i := uint(0); i < conf.PoolSize; i++ {
		ctx.Pool.Add(1)
		go Worker(ctx, conf, WorkerID(i))
	}

	ctx.Pool.Add(1)
	go Watcher(ctx, conf)

	// now just wait for the signal to stop.  this is either a ctrl+c
	// or a SIGTERM.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	signal.Notify(sigChan, syscall.SIGTERM)
stopLoop:
	for {
		select {
		case <- sigChan:
			log.Printf("termination requested...\n")
			break stopLoop
			
		case threadMsg := <- ctx.Control:
			if threadMsg == ThreadCannotContinue {
				log.Print("thread error!  cannot continue...")
				break stopLoop
			}
		}
	}

	// Close everybody gracefully
	ctx.Control <- StopExecution
	close(ctx.FilePipeline)
	ctx.Pool.Wait()

	log.Print("All goroutines finished.  terminating...")
}
