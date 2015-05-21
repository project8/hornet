/*
* hornet - a tool for nearline processing of Project 8 triggered data
*
*  https://github.com/project8/hornet
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

	// ThreadCannotContinue signals that the sending thread cannot continue
	// executing due to an error, and hornet should shut down.
	ThreadCannotContinue = 1
)

// Config represents the user-specified configuration data of hornet
type Config struct {
        ScheduleQueueSize  uint
	PoolSize           uint
	WatchDirPath       string
	DestDirPath        string
	KatydidPath        string
	KatydidConfPath    string
}

// Validate checks the sanity of a Config instance
//   1) the number of threads is sane
//   2) the provided path to katydid actually points at an executable
//   3) the provided config file is parsable json
//   4) the provided watch directory is indeed a directory
func (c Config) Validate() (e error) {
        fmt.Println("[hornet] Scheduler queue size:", c.ScheduleQueueSize)
        fmt.Println("[hornet] Watcher directory:", c.WatchDirPath)
        fmt.Println("[hornet] Pool size:", c.PoolSize)
        fmt.Println("[hornet] Katydid path:", c.KatydidPath)
        fmt.Println("[hornet] Katydid config path:", c.KatydidConfPath)
        fmt.Println("[hornet] Destination directory:", c.DestDirPath)

        if c.ScheduleQueueSize == 0 {
                e = fmt.Errorf("Scheduler queue must be greater than 0")
        }

	if c.PoolSize > MaxPoolSize || c.PoolSize == 0 {
		e = fmt.Errorf("Size of thread pool must be greater than 0 and cannot exceed %d", MaxPoolSize)
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

	if PathIsDirectory(c.DestDirPath) == false {
		e = fmt.Errorf("Destination directory must exist and be a directory!")
	}

	if PathIsDirectory(c.WatchDirPath) == false {
		e = fmt.Errorf("Watch directory must exist and be a directory!")
	}

	return
}

func main() {
	// user needs help
	var needHelp bool

	// default configuration parameters.  these may be changed by
	// command line options.
	config := Config{}

        // configuration file
        var configFile string

	// set up flag to point at conf, parse arguments and then verify
	flag.BoolVar(&needHelp,
		"help",
		false,
		"display this dialog")
        flag.StringVar(&configFile,
                "config",
                "",
                "JSON configuration file")
        flag.UintVar(&config.ScheduleQueueSize,
                "schedule-queue",
                0,
                "size of the scheduler queue")
	flag.UintVar(&config.PoolSize,
		"pool-size",
		0,
		"size of worker pool")
	flag.StringVar(&config.KatydidPath,
		"katydid-path",
		"",
		"full path to Katydid executable")
	flag.StringVar(&config.KatydidConfPath,
		"katydid-conf",
		"",
		"full path to Katydid config file to use when processing")
	flag.StringVar(&config.WatchDirPath,
		"watch-dir",
		"",
		"directory to watch for new data files")
	flag.StringVar(&config.DestDirPath,
		"dest-dir",
		"",
		"directory to move files to when finished")
	flag.Parse()

	if needHelp {
		flag.Usage()
		os.Exit(1)
	} 
        
        // Parse the config file if one has been supplied
        // Then, check for each section of the config file and use the values if 
        // the corresponding information was not provided on the CLI
        if len(configFile) != 0 {
                configBytes, confReadErr := ioutil.ReadFile(configFile)

                if confReadErr != nil {
                        log.Fatal("(FATAL) ", confReadErr)
                }

                var configValues interface{}
                jsonErr := json.Unmarshal(configBytes, &configValues)
                if jsonErr != nil {
                        log.Fatal("(FATAL) ", jsonErr)
                }

                configMap := configValues.(map[string]interface{})

                schedulerValues, schedValsFound := configMap["scheduler"]
                if schedValsFound {
                        schedulerMap := schedulerValues.(map[string]interface{})
                        if value, valueFound := schedulerMap["queue"]; valueFound && config.ScheduleQueueSize == 0 {
                                config.ScheduleQueueSize = uint(value.(float64))
                        }
                }

                watcherValues, watchValsFound := configMap["watcher"]
                if watchValsFound {
                        watcherMap := watcherValues.(map[string]interface{})
                        if value, valueFound := watcherMap["dir"]; valueFound && len(config.WatchDirPath) == 0 {
                                config.WatchDirPath = value.(string)
                        }
                }

                workerValues, workValsFound := configMap["workers"]
                if workValsFound {
                        workerMap := workerValues.(map[string]interface{})
                        if value, valueFound := workerMap["pool-size"]; valueFound && config.PoolSize == 0 {
                                config.PoolSize = uint(value.(float64))
                        }
                        if value, valueFound := workerMap["katydid-path"]; valueFound && len(config.KatydidPath) == 0 {
                                config.KatydidPath = value.(string)
                        }
                        if value, valueFound := workerMap["katydid-config"]; valueFound && len(config.KatydidConfPath) == 0 {
                                config.KatydidConfPath = value.(string)
                        }
                }

                moverValues, moveValsFound := configMap["mover"]
                if moveValsFound {
                        moverMap := moverValues.(map[string]interface{})
                        if  value, valueFound := moverMap["dest"]; valueFound && len(config.DestDirPath) == 0 {
                                config.DestDirPath = value.(string)
                        }
                }
        }
                        
	if configErr := config.Validate(); configErr != nil {
               	flag.Usage()
		log.Fatal("(FATAL) ", configErr)
	}


	// if we've made it this far, it's time to get down to business.

	var pool sync.WaitGroup

        schedulingQueue := make(chan string, 100)
        controlQueue := make(chan ControlMessage)
        requestQueue := make(chan ControlMessage)

        // check to see if any files are being scheduled via the command line
        for iFile := 0; iFile < flag.NArg(); iFile++ {
                fmt.Println("Scheduling", flag.Arg(iFile))
                schedulingQueue <- flag.Arg(iFile)
        }

        pool.Add(1)
        go Scheduler(schedulingQueue, controlQueue, requestQueue, config.PoolSize, &pool, config)

	// now just wait for the signal to stop.  this is either a ctrl+c
	// or a SIGTERM.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	signal.Notify(sigChan, syscall.SIGTERM)
stopLoop:
	for {
		select {
		case <-sigChan:
			log.Printf("[hornet] termination requested...\n")
			break stopLoop

		case threadMsg := <-requestQueue:
			if threadMsg == ThreadCannotContinue {
				log.Print("[hornet] thread error!  cannot continue...")
				break stopLoop
			}
		}
	}

	// Close all of the worker threads gracefully
	for i := 0; i < 3; i++ {
		controlQueue <- StopExecution
	}
	pool.Wait()

	log.Print("[hornet] All goroutines finished.  terminating...")
}
