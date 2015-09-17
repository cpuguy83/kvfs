FROM golang:1.5
ADD . /go/src/github.com/cpuguy83/kvfs
WORKDIR /go/src/github.com/cpuguy83/kvfs
RUN go get github.com/tools/godep
RUN godep get
RUN godep go build && cp kvfs /usr/local/bin/
