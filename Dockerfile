FROM golang:1.23.9

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
    mkdir /data/hot0 && \
    mkdir /data/hot1 && \
    mkdir /data/hot2 && \
    mkdir /data/warm.julius && \
    mkdir /data/warm && \
    mkdir /data/cold

# This next is a hack, it requires you to have first done ``cp ~/.project8_authentications project8_authentications``
# There is probably a data-volumes based solution that cleans this up

RUN cd /go/src/github.com/project8/hornet && go install

# Create some test files for transfer

RUN touch /data/hot0/test1.Setup
RUN touch /data/hot0/test2.Setup
RUN touch /data/hot0/test3.Setup

CMD ["hornet", "-config", "/config/hornet_config.json"]
