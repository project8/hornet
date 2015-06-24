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
  > go get gopkg.in/niemeyer/godeb.v1/cmd/godeb
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
* [uuid](https://code.google.com/p/go-uuid/uuid) for getting UUIDs (note that you may need Mercurial on your system to get this package);
* [viper](https://github.com/spf13/viper) for the application configuration.
```
  > go get github.com/streadway/amqp
  > go get github.com/ugorji/go/codec
  > go get github.com/op/go-logging
  > go get golang.org/x/exp/inotify
  > go get github.com/kardianos/osext
  > go get code.google.com/p/go-uuid/uuid
  > go get github.com/spf13/viper
```

#### Operating system support
Because Hornet uses inotify, currently it will only build correctly on Linux
systems.  In the future it would be lovely to support OS X as well, but that will
have to wait until fsevents support comes along.  If this is really necessary,
an OS X-only build which uses simple filesystem polling may be implemented to take
care of this.


### Installation
Download hornet:
```
  > go get github.com/project8/hornet
```

Update hornet's knowledge of its git commit and tag:
```
  > make remove_older_describe_go
```

There are two options for building hornet:

1. If you want to build an "official" copy that gets installed for general use:

        > go install hornet

 The executable `hornet` should now be in `$GOPATH/bin`.
2. If you want to build a local copy for development purposes:

        > go build -o run_hornet .

 This will create the executable `run_hornet` in the source directory. The name is different from `hornet` since there's a subdirectory with that name.


## Running hornet
1. Make a copy of `examples/hornet_config.json`
2. Make any changes needed.  At a minimum, you will need to change `amqp.broker` (assuming either the sender or receiver are active), `watcher.dir` (assuming the watcher is active), `mover.dest-dir`, and `shipper.dest-dir`.  All of the directories used must exist.
3. Run the executable:

        > hornet --config my_config.json

### So, you want to . . .
* Change the number of nearline workers to adapt to available processing power and analysis time: `scheduler.n-nearline-workers`
* Turn AMQP usage off: `amqp.receiver` and `amqp.sender` set to `false`.  Also, if `hash.required` is `true`, then make sure `hash.send-to` is `""` (empty string)
* Turn the watcher on or off: `watcher.active` set to `true` or `false`, respectively
* Set the directory for the watcher: `watcher.dir`
* Add/remove/modify a recognized file type: `classifier.types.[whatever]`
* Change the warm data storage: `mover.dest-dir`
* Change the cold data storage: `shipper.dest-dir`
