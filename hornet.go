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
	"time"

	"github.com/spf13/viper"

	"github.com/project8/hornet/hornet"
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

	// Threads used:
	//   1 each for the scheduler, classifier, watcher, mover, amqp sender, amqp receiver = 6
	//   N nearline workers (specified in scheduler.n-nearline-workers)
	//   M shippers (specified in scheduler.n-shippers)
	nThreads := 6 + viper.GetInt("scheduler.n-nearline-workers") + viper.GetInt("scheduler.n-shippers")
	if nThreads > hornet.MaxThreads {
		e = fmt.Errorf("Maximum number of threads exceeded")
	}

	if amqpErr := hornet.ValidateAmqpConfig(); amqpErr != nil {
		log.Print(amqpErr.Error())
		e = amqpErr
	}

	if classifierErr := hornet.ValidateClassifierConfig(); classifierErr != nil {
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

	if hornet.PathIsDirectory(viper.GetString("mover.dest-dir")) == false {
		e = fmt.Errorf("Destination directory must exist and be a directory!")
	}

	if hornet.PathIsDirectory(viper.GetString("watcher.dir")) == false {
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
	controlQueue := make(chan hornet.ControlMessage)
	requestQueue := make(chan hornet.ControlMessage)
	threadCountQueue := make(chan uint, hornet.MaxThreads)

	hornet.StartAmqp(controlQueue, requestQueue, threadCountQueue, &pool)

	// check to see if any files are being scheduled via the command line
	for iFile := 0; iFile < flag.NArg(); iFile++ {
		fmt.Println("Scheduling", flag.Arg(iFile))
		schedulingQueue <- flag.Arg(iFile)
	}

	pool.Add(1)
	threadCountQueue <- 1
	go hornet.Scheduler(schedulingQueue, controlQueue, requestQueue, threadCountQueue, &pool)

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

		case requestMsg := <-requestQueue:
			switch requestMsg {
			case hornet.ThreadCannotContinue:
				log.Print("[hornet] thread error!  cannot continue running")
				break stopLoop
			case hornet.StopExecution:
				log.Print("[hornet] stop-execution request received")
				break stopLoop
			}
		}
	}

	// Close all of the worker threads gracefully
	// Use the select/default idiom to avoid the problem where one of the threads has already
	// closed and we can't send to the control queue
	log.Printf("[hornet] stopping %d threads", len(threadCountQueue)-1)
	for i := 0; i < len(threadCountQueue); i++ {
		select {
		case controlQueue <- hornet.StopExecution:
		default:
		}
	}

	// Timed call to pool.Wait() in case one or more of the threads refuses to close
	// Use the channel-based concurrency pattern (http://blog.golang.org/go-concurrency-patterns-timing-out-and)
	// We have to wrap pool.Wait() in a go routine that sends on a channel
	waitChan := make(chan bool, 1)
	go func() {
		pool.Wait()
		waitChan <- true
	}()
	select {
	case <-waitChan:
		log.Print("[hornet] All goroutines finished.")
	case <-time.After(1 * time.Second):
		log.Print("[hornet] Timed out waiting for goroutines to finish.")
	}
}
