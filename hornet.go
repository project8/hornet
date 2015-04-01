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
	"sync"
)

// Global config
var (
	MaxPoolSize uint = 25
)

// Config represents the user-specified configuration data of hornet
type Config struct {
	PoolSize        uint
	KatydidPath     string
	KatydidConfPath string
}

// Validate checks the sanity of a Config instance
//   1) the number of threads is sane
//   2) the provided path to katydid actually points at an executable
//   3) the provided config file is parsable json
func (c Config) Validate() (e error) {
	if c.PoolSize > MaxPoolSize {
		e = fmt.Errorf("Size of thread pool cannot exceed %d!", MaxPoolSize)
	}

	if execInfo, execErr := os.Stat(c.KatydidPath); execErr == nil {
		if (execInfo.Mode() & 0111) == 0 {
			e = fmt.Errorf("Katydid does not appear to be executable!")
		}
	} else {
		e = fmt.Errorf("error when examining Katydid executable: %v",
			execErr)
	}

	if confBytes, confErr := ioutil.ReadFile(c.KatydidConfPath); 
	confErr == nil {
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

	return
}

// Context is the state of hornet
type Context struct {
	Pool *sync.WaitGroup
	FilePipeline chan string
}

func main() {
	// default configuration parameters.  these may be changed by
	// command line options.
	conf := Config{}

	// set up flag to point at conf, parse arguments and then verify
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
	flag.Parse()

	if configErr := conf.Validate(); configErr != nil {
		flag.Usage()
		log.Fatal("(FATAL) ", configErr)
	}

	// if we've made it this far, it's time to get down to business.
	// we'll start an inotify Notify request for the working directory.
	// then we have a single channel that we use to communicate with the
	// worker pool.  as we get file closed events, we check to see if we
	// already know about the file in question.  if not, and it is a
	// data file, we'll enqueue it to get chewed on by the next available
	// worker thread.
	var pool sync.WaitGroup
	ctx := Context{
		FilePipeline: make(chan string),
		Pool: &pool,
	}

	for i := uint(0); i < conf.PoolSize; i++ {
		ctx.Pool.Add(1)
		go Worker(ctx, conf)
	}

	wd, wdErr := os.Open(".")
	if wdErr != nil {
		log.Fatal("couldn't open working directory!")
	}

	fnames, _  := wd.Readdirnames(0)
	for _, fname := range(fnames) {
		ctx.FilePipeline <- fname
	}
	
	close(ctx.FilePipeline)
	ctx.Pool.Wait()
	log.Print("All goroutines finished.  terminating...")
}
