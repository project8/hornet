FROM golang:latest

# install requisite debian components
RUN apt-get update && apt-get install -y rsync
# install development debian components (comment this out for minimal/production)
RUN apt-get install -y vim \
                       tree \
                       less

# add the local source and build the application
ADD . /go/src/github.com/project8/hornet

# Make some directories and files
RUN echo "{\n}" > ~/.project8_authentications.json && \
    mkdir /data && \
    mkdir /data/hot && \
    mkdir /data/warm && \
    mkdir /data/cold && \
    # modify hornet config to be purely local
    sed -e '9s/true/false/' \
        # disable slack
        -e '19s/true/false/' \
        # use the directory names above for storage
        -e 's@"dir": "/data"@"dir": "/data/hot"@' \
        -e 's@"/warm-data"@"/data/warm"@' \
        # no amqp connection so disable sending file info
        -e 's@"send-file-info": true@"send-file-info": false@' \
        # shipper config to just local move
        -e 's@"/remote-data"@"/data/cold"@' \
        -e '101s/,//' \
        -e '102,103d' \
        # remove base-paths
        -e '59,60d' \
        # remove estra dirs from watcher
        -e '30,34d' \
        -e '29s/,//' \
        /go/src/github.com/project8/hornet/examples/hornet_config.json > /go/hornet_config.json

RUN cat /go/hornet_config.json

# use go to install golang deps
#RUN go install github.com/streadway/amqp@latest \
#           github.com/ugorji/go/codec@latest \
#           github.com/op/go-logging@latest \
#           golang.org/x/exp/inotify@latest \
#           github.com/kardianos/osext@latest \
#           code.google.com/p/go-uuid/uuid@latest \
#           github.com/spf13/viper@latest

# This next is a hack, it requires you to have first done ``cp ~/.project8_authentications project8_authentications``
# There is probably a data-volumes based solution that cleans this up

RUN cd /go/src/github.com/project8/hornet && make remove_older_describe_go && make all

RUN go install github.com/project8/hornet@latest

CMD ["hornet", "-config", "hornet_config.json"]
