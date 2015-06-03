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
Hornet requires go version 1.1 or better.  It's recommended that you setup your  go workspace and `GOPATH` environment ([e.g.](http://golang.org/doc/code.html#Workspaces)) in the standard way.

For use on systems where the standard go version is too old (e.g. Debian Wheezy), 
the `godeb` application is suggested.  First, install the too-old version of `golang`.
Then proceed to install godeb.  When I (Noah) installed it on teselecta, I used the following sequence of commands:
```
  > go get gopkg.in/niermyer/godeb.v1/cmd/godeb
  > sudo apt-get remove golang
  > sudo $GOPATH/bin/godeb install
  > sudo dpkg -i --force-overwrite go_[version]-godeb1_[system].deb
```
The second line removes the old version of go that I previously installed using the apt-get package manager.
The third line gave me an error, so I used the fourth line to force the package manager to overwrite 
the golang package.

Aside from standard go libraries, several external packages are used, which you'll need to acquire:
* [amqp](https://github.com/streadway/amqp) for sending and receiving AMQP messages;
* [codec](https://github.com/ugorji/go/codec) for encoding and decoding JSON and msgpack;
* [inotify](https://golang.org/x/exp/inotify) for tracking file system events (Linux only);
* [osext](https://github.com/kardianos/osext) for finding the absolute executable path in a platform-independent way;
* [viper](https://github.com/spf13/viper) for the application configuration.
```
  > go get github.com/streadway/amqp
  > go get github.com/ugorji/go/codec
  > go get golang.org/x/exp/inotify
  > go get github.com/kardianos/osext
  > go get github.com/spf13/viper
```

#### Operating system support
Because Hornet uses inotify, currently it will only build correctly on Linux
systems.  In the future it would be lovely to support OS X as well, but that will
have to wait until fsevents support comes along.  If this is really necessary,
an OS X-only build which uses simple filesystem polling may be implemented to take
care of this.


### Installation
We suggest that you create a build directory to keep the built files out of the source tree:
```
  > mkdir build
  > cd build
```

Then use the `go` tool to build Hornet:
```
  > go build -o hornet ../*.go
```

This should create the executable `hornet`. 

## Running hornet
To run hornet at the command line, you must supply a few parameters:
* Path to your Katydid executable
* Path to the Katydid config file you want to have run by Hornet
* The maximum number of workers (i.e. threads which are running Katydid)
* The directory to watch for new files.

Check ```hornet -h``` for the most up-to-date invocation rules and syntax.
