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
	//"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/viper"

	"github.com/project8/hornet/hornet"
)


func main() {
	// setup logging, first thing
	hornet.InitializeLogging()

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

	fmt.Println("         _       _    _            _           _             _          _")
	fmt.Println("        / /\\    / /\\ /\\ \\         /\\ \\        /\\ \\     _    /\\ \\       /\\ \\")
	fmt.Println("       / / /   / / //  \\ \\       /  \\ \\      /  \\ \\   /\\_\\ /  \\ \\      \\_\\ \\")
	fmt.Println("      / /_/   / / // /\\ \\ \\     / /\\ \\ \\    / /\\ \\ \\_/ / // /\\ \\ \\     /\\__ \\")
	fmt.Println("     / /\\ \\__/ / // / /\\ \\ \\   / / /\\ \\_\\  / / /\\ \\___/ // / /\\ \\_\\   / /_ \\ \\")
	fmt.Println("    / /\\ \\___\\/ // / /  \\ \\_\\ / / /_/ / / / / /  \\/____// /_/_ \\/_/  / / /\\ \\ \\")
	fmt.Println("   / / /\\/___/ // / /   / / // / /__\\/ / / / /    / / // /____/\\    / / /  \\/_/")
	fmt.Println("  / / /   / / // / /   / / // / /_____/ / / /    / / // /\\____\\/   / / /")
	fmt.Println(" / / /   / / // / /___/ / // / /\\ \\ \\  / / /    / / // / /______  / / /")
	fmt.Println("/ / /   / / // / /____\\/ // / /  \\ \\ \\/ / /    / / // / /_______\\/_/ /")
	fmt.Println("\\/_/    \\/_/ \\/_________/ \\/_/    \\_\\/\\/_/     \\/_/ \\/__________/\\_\\/\n")

	hornet.Log.Notice("Reading config file: %v", configFile)
	viper.SetConfigFile(configFile)
	if parseErr := viper.ReadInConfig(); parseErr != nil {
		hornet.Log.Critical("%v", parseErr)
	}

	// configure the logger, now that the config file is ready
	hornet.ConfigureLogging()

	// print the full configuration
	indentedConfig, confErr := json.MarshalIndent(viper.AllSettings(), "", "    ")
	if confErr != nil {
		hornet.Log.Critical("Error marshaling configuration!")
		return
	}
	hornet.Log.Debug("Full configuration:\n%v", string(indentedConfig))

	if viper.GetBool("amqp.active") || viper.GetBool("slack.active") {
		// get the authenticator credentials
		if authErr := hornet.LoadAuthenticators(); authErr != nil {
			hornet.Log.Critical("Error getting authentication credentials:\n\t%s", authErr.Error())
			return
		}
	}

	// Check the number of threads to be used
	// Threads used:
	//   1 each for the scheduler, classifier, watcher, mover, amqp sender, amqp receiver, slack client = 7
	//   N nearline workers (specified in scheduler.n-nearline-workers)
	//   M shippers (specified in scheduler.n-shippers)
	nThreads := 7 + viper.GetInt("workers.n-workers") + viper.GetInt("shipper.n-shippers")
	if nThreads > hornet.MaxThreads {
		hornet.Log.Critical("Maximum number of threads exceeded")
		return
	}

	var pool sync.WaitGroup

	// get the queue size from the configuration, and increase it if we need to handle lots of files
	queueSize := viper.GetInt("scheduler.queue-size")
	if flag.NArg() > queueSize {
		queueSize = flag.NArg()
		viper.Set("scheduler.queue-size", queueSize)
	}

	schedulingQueue := make(chan string, queueSize)
	controlQueue := make(chan hornet.ControlMessage)
	requestQueue := make(chan hornet.ControlMessage)
	threadCountQueue := make(chan uint, hornet.MaxThreads)

	// Setup the connection to slack
	if slackErr := hornet.InitializeSlack(controlQueue, requestQueue, threadCountQueue, &pool); slackErr != nil {
		hornet.Log.Critical("Error initializing slack: %v", slackErr.Error())
		return
	}

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
			hornet.Log.Notice("Termination requested...\n")
			break stopLoop

		case requestMsg := <-requestQueue:
			switch requestMsg {
			case hornet.ThreadCannotContinue:
				hornet.Log.Notice("Thread error!  Cannot continue running")
				break stopLoop
			case hornet.StopExecution:
				hornet.Log.Notice("Stop-execution request received")
				break stopLoop
			}
		}
	}

	// Close all of the worker threads gracefully
	// Use the select/default idiom to avoid the problem where one of the threads has already
	// closed and we can't send to the control queue
	hornet.Log.Info("Stopping %d threads", len(threadCountQueue)-1)
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
		hornet.Log.Info("All goroutines finished.")
	case <-time.After(1 * time.Second):
		hornet.Log.Info("Timed out waiting for goroutines to finish.")
	}
}
