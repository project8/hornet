FROM golang:1.4

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
    sed -e '9s/true/false/' \
        -e '19s/true/false/' \
        -e 's@"dir": "/data"@"dir": "/data/hot"@' \
        -e 's@"/warm-data"@"/data/warm"@' \
        -e 's@"send-file-info": true@"send-file-info": false@' \
        -e 's@"/remote-data"@"/data/cold"@' \
        -e '101s/,//' \
        -e '102,103d' \
        -e '59,60d' \
        -e '30,34d' \
        -e '29s/,//' \
        /go/src/github.com/project8/hornet/examples/hornet_config.json > /go/hornet_config.json

# use go to install golang deps
RUN go get github.com/streadway/amqp \
           github.com/ugorji/go/codec \
           github.com/op/go-logging \
           golang.org/x/exp/inotify \
           github.com/kardianos/osext \
           code.google.com/p/go-uuid/uuid \
           github.com/spf13/viper

# This next is a hack, it requires you to have first done ``cp ~/.project8_authentications project8_authentications``
# There is probably a data-volumes based solution that cleans this up

RUN cd /go/src/github.com/project8/hornet && make remove_older_describe_go

RUN go install github.com/project8/hornet
