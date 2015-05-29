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
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/spf13/viper"
)

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

// Validate checks the sanity of a Config instance
//   1) the number of threads is sane
//   2) the provided config file is parsable json
//   3) the provided watch directory is indeed a directory
func ValidateConfig() (e error) {
	indentedConfig, confErr := json.MarshalIndent(viper.AllSettings(), "", "    ")
	if confErr != nil {
		e = fmt.Errorf("Error marshaling configuration!")
		return
	}
	log.Printf("[hornet] Full configuration:\n%v", string(indentedConfig))

	nThreads := 1 /*scheduler*/ + 1 /*watcher*/ + 1 /*mover*/ + viper.GetInt("scheduler.n-nearline-workers") + viper.GetInt("scheduler.n-shippers")
	if nThreads > MaxThreads {
		e = fmt.Errorf("Maximum number of threads exceeded")
	}

	if amqpErr := ValidateAmqpConfig(); amqpErr != nil {
		log.Print(amqpErr.Error())
		e = amqpErr
	}

	if classifierErr := ValidateClassifierConfig(); classifierErr != nil {
		log.Print(classifierErr.Error())
		e = classifierErr
	}

	if viper.GetInt("scheduler.n-nearline-workers") == 0 {
		e = fmt.Errorf("Cannot have 0 nearline workers")
	}

	// for now, we require that there's only 1 shipper
	if viper.GetInt("scheduler.n-shippers") != 1 {
	}

	if viper.GetInt("scheduler.queue-size") == 0 {
		e = fmt.Errorf("Scheduler queue must be greater than 0")
	}

	if PathIsDirectory(viper.GetString("mover.dest-dir")) == false {
		e = fmt.Errorf("Destination directory must exist and be a directory!")
	}

	if PathIsDirectory(viper.GetString("watcher.dir")) == false {
		e = fmt.Errorf("Watch directory must exist and be a directory!")
	}

	return
}

func main() {
	// user needs help
	var needHelp bool

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
	flag.Parse()

	if needHelp {
		flag.Usage()
		os.Exit(1)
	}

	log.Print("[hornet] Reading config file: ", configFile)
	viper.SetConfigFile(configFile)
	if parseErr := viper.ReadInConfig(); parseErr != nil {
		log.Fatal("(FATAL) ", parseErr)
	}

	//fmt.Println(viper.AllSettings())

	if configErr := ValidateConfig(); configErr != nil {
		flag.Usage()
		log.Fatal("(FATAL) ", configErr)
	}

	// if we've made it this far, it's time to get down to business.

	var pool sync.WaitGroup

	schedulingQueue := make(chan string, viper.GetInt("scheduler.queue-size"))
	controlQueue := make(chan ControlMessage)
	requestQueue := make(chan ControlMessage)
	threadCountQueue := make(chan uint, MaxThreads)

	StartAmqp(controlQueue, requestQueue, threadCountQueue, &pool)

	// check to see if any files are being scheduled via the command line
	for iFile := 0; iFile < flag.NArg(); iFile++ {
		fmt.Println("Scheduling", flag.Arg(iFile))
		schedulingQueue <- flag.Arg(iFile)
	}

	pool.Add(1)
	threadCountQueue <- 1
	go Scheduler(schedulingQueue, controlQueue, requestQueue, threadCountQueue, &pool)

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
	log.Printf("[hornet] stopping %d threads", len(threadCountQueue)-1)
	for i := 0; i < len(threadCountQueue); i++ {
		controlQueue <- StopExecution
	}
	pool.Wait()

	log.Print("[hornet] All goroutines finished.  terminating...")
}
