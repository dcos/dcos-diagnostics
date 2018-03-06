FROM golang:1.9.3

ENV PATH /go/bin:/usr/local/go/bin:$PATH
ENV GOPATH /go

RUN apt-get update && apt-get install -y \
    libsystemd-dev

RUN go get github.com/stretchr/testify
