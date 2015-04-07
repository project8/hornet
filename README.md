# Hornet

Hornet is a nearline data processing engine written in Go.  Its purpose
is to provide rapid feedback about the content of data that is being produced
by a triggered DAQ system such as the Tektronix RSA.  

Hornet uses inotify to watch for filesystem changes on a specific directory.
When a file close event is detected, it checks to see if the file should be
processed (currently it simply checks if it is a MAT file such as those produced
by the RSA), and if it does the file is scheduled for processing.  

A simple FCFS scheduler is applied to incoming events.  A group of
goroutines is waiting for new filenames to be processed, and as they come in the
first free goroutine takes the job of processing the data.  

The worker simply invokes Katydid with a config file specified on the command
line at runtime.  

### Dependencies
Hornet is almost all standard library, except for the inotify package at 
golang.org[golang.org/x/exp/inotify].  You only need the ```go``` tool to build
Hornet and run it.  

Because Hornet uses inotify, currently it will only build correctly on Linux
systems.  In the future it would be lovely to support OSX as well, but that will
have to wait until fsevents support comes along.  If this is really necessary,
an OSX-only build which uses simple filesystem polling may be implemented to take
care of this.

## Running hornet
To run hornet at the command line, you must supply a few parameters:
* Path to your Katydid executable
* Path to the Katydid config file you want to have run by Hornet
* The maximum number of workers (i.e. threads which are running Katydid)
* The directory to watch for new files.

Check ```hornet -h``` for the most up-to-date invocation rules and syntax.
