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
* [fsnotify](https://gopkg.in/fsnotify.v1) for accessing file-system events;
* [go-logging](https://) for nice color-coded logging;
* [osext](https://github.com/kardianos/osext) for finding the absolute executable path in a platform-independent way;
* [uuid](https://code.google.com/p/go-uuid/uuid) for getting UUIDs (note that you may need Mercurial on your system to get this package);
* [viper](https://github.com/spf13/viper) for the application configuration.
```
  > go get github.com/streadway/amqp
  > go get github.com/ugorji/go/codec
  > go get gopkg.in/fsnotify.v1
  > go get github.com/op/go-logging
  > go get github.com/kardianos/osext
  > go get code.google.com/p/go-uuid/uuid
  > go get github.com/spf13/viper
```

When using on a Windows system, Hornet requires the [Microsoft File Checksum Integrity Verifier](http://www.microsoft.com/en-us/download/details.aspx?id=11533). Please make sure the install location is included in the `Path` environment variable.

#### Operating system support
Hornet has been tested on Linux (Debian 8), Mac (OS X 10.10), and Windows (7).


### Installation
Download hornet:
```
  > go get -b github.com/project8/hornet
```
Add the -u flag to update hornet and its dependencies.

Update hornet's knowledge of its git commit and tag:
```
  > cd $GOPATH/src/github.com/project8/hornet
  > make remove_older_describe_go
```

There are two options for building hornet:

1. If you want to build an "official" copy that gets installed for general use:
```
  > go install github.com/project8/hornet
```

 The executable `hornet` should now be in `$GOPATH/bin`.
2. If you want to build a local copy for development purposes:
```
  > go build -o run_hornet .
```

 This will create the executable `run_hornet` in the source directory. The name is different from `hornet` since there's a subdirectory with that name.


## Running hornet
1. Make a copy of `examples/hornet_config.json`
2. Make any changes needed.  At a minimum, you will need to change `amqp.broker` (assuming either the sender or receiver are active), `watcher.dir` (assuming the watcher is active), `mover.dest-dir`, and `shipper.dest-dir`.  All of the directories used must exist.
3. Run the executable:
```
  > hornet --config my_config.json
```

### So, you want to . . .
Unless otherwise noted, these are config file values to set.
* Submit files directly to hornet for processing (command line): `hornet --config my_config.json [files to be processed; wildcards allowed]`
* Change the number of nearline workers to adapt to available processing power and analysis time: `scheduler.n-nearline-workers`
* Turn AMQP usage off: `amqp.receiver` and `amqp.sender` set to `false`.  Also, if `hash.required` is `true`, then make sure `hash.send-to` is `""` (empty string)
* Turn the watcher on or off: `watcher.active` set to `true` or `false`, respectively
* Set the directory for the watcher: `watcher.dir`
* Add/remove/modify a recognized file type: `classifier.types.[whatever]`
* Change the warm data storage: `mover.dest-dir`
* Change the cold data storage: `shipper.dest-dir`


### Docker
In addition to the instructions which follow for a "conventional" installation, it is also possible to run hornet using docker.
This feature is still in development, but should be completely independent of platform.
In particular, the linux implementation of hornet should be usable on windows using this virtualization.

##### Installation/dependencies
1. **Install Docker:** the details are highly system dependent and docker maintains excellent [instructions online](http://docs.docker.com/) (from the left menu, select "Install > Docker Engine" then your operating system). In the case of debian jessie, you simply install the docker.io package from backports (requires adding a new source to apt). For Windows or Mac you need to install boot2docker. Obviously this should only need to be done once.
2. **Clone Hornet:** This is just as trivial as always, ``git clone git@github.com:project8/hornet``
3. **Build the Docker Image:** The simplest example is to change into the top-level hornet directory (where the "Dockerfile" is located) and run ``docker build .``. You probably will want to include a name and tag for your image ``docker build -t hornet_default:latest .`` and possibly further options which I haven't learned about yet.

##### Running a container
I'll assume you've built the image above (ie with name hornet_default and tag latest).
You can, in principle, simply spin up the container and have hornet running in a background container ``docker -d hornet:latest``.
You should read over the docker documentation for complete details, but I'll outline a few important options quickly:
- if you use ``-it`` the container runs interactively in a sudo terminal (ie you'll be attached to hornet)
- if you use ``--rm`` the container will be deleted once it exits
- if you use ``--name <name>``, you can assign a name to the container for reference later
- if you use ``docker exec -it <name> bash``, you will attach to a terminal in your running container (without disrupting hornet)

##### Configuration of a hornet container
By default, when you spin up the container it runs a basic (an not very useful) instance of hornet.
This is to keep the container image portable, but means that it doesn't know anything about the host system or other resources available.
In all cases, you can expand features using docker volumes.
1. **Configuration:** If you want to modify the default configuration, create a local configuration file and execute your docker run with an extra ``-v </path/on/host/to/config.json>:/go/hornet_config.json`` argument.
2. **Authentication:** For reasons which are hopefully obvious, an authentication file is not provided in the container. As with the configuration file, ``-v /home/<user>/.project8_authentications.json:/root/.project8_authentications.json`` (or whatever non-standard path options you may want to use).
3. **File directories:** Probably most important, you can use the same ``-v`` to mount whatever directories you want to watch and/or write to. If you forget to do this, the targets will be within the containers filesystem (and probably not useful). If you don't use your own configuration file, you would mount over /data/[hot|warm|cold] with whatever local directories you like.

##### Warnings
There are several important things left to test/develop that I'll caution you about here:
1. I haven't done any performance tests of this vs native hornet. In the case of Mac/Windows, boot2docker invovles running a linux VM to host the docker engine, which may have some impact.
2. Because Windows and Mac require a VM, host filesystem access is less straight forward. I have not yet tested mounting other drives to the container. I expect that this should be possible, but the details are not yet clear.
3. Similar to the above, all testing has been done on a container not doing any external communication. There may be issues with trying to run with active communication to AMQP, slack, or other network systems. Docker is, in general, made to be able to do this, so it shouldn't be hard, but again the details haven't been worked through.
4. Finally, I have not yet tried to use a container with bbcp or gridFTP via globus. The first case requires installing the tool which probably means I should post different base image to dockerhub that includes it. The second case requires an endpoint be registered, which requires an account with globus and generating a key. We don't want to have to do this every time so we'll need to think it through a bit.
