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
	"flag"
	"log"
	"fmt"
	"os"
	"io/ioutil"
	"bytes"
	"encoding/json"
)

// Global config
var (
	MaxPoolSize uint = 25
)

// Configuration data.  Changeable from the command line.
type Config struct {
	PoolSize uint
	KatydidPath string
	KatydidConfPath string
}

// validate the config of hornet.  this means that:
//   1) the number of threads is sane
//   2) the provided path to katydid actually points at an executable
//   3) the provided config file is parsable json
func (c Config) Validate() (e error) {
	if c.PoolSize > MaxPoolSize {
		e = fmt.Errorf("Size of thread pool cannot exceed %d!",MaxPoolSize)
	}

	if exec_info, exec_err := os.Stat(c.KatydidPath); exec_err == nil {
		if (exec_info.Mode() & 0111) == 0 {
			e = fmt.Errorf("Katydid does not appear to be executable!")
		}
	} else {
		e = fmt.Errorf("error when examining Katydid executable: %v",
			exec_err)
	}

	if conf_bytes, conf_err := ioutil.ReadFile(c.KatydidConfPath); conf_err == nil {
		v := make(map[string]interface{})
		conf_decoder := json.NewDecoder(bytes.NewReader(conf_bytes))
		if decode_err := conf_decoder.Decode(&v); decode_err != nil {
			e = fmt.Errorf("Katydid config appears unparseable... %v",
				decode_err)
		}
	} else {
		e = fmt.Errorf("error when opening Katydid config: %v",
			conf_err)
	}
	
	return
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

	if config_err := conf.Validate(); config_err != nil {
		flag.Usage()
		log.Fatal("(FATAL) ",config_err)
	}
	
}
