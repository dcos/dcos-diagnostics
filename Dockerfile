FROM golang:1.11.0

ENV PATH /go/bin:/usr/local/go/bin:$PATH
ENV GOPATH /go

RUN apt-get update && apt-get install -y \
    libsystemd-dev

RUN go get github.com/stretchr/testify
RUN go get github.com/alecthomas/gometalinter
RUN gometalinter --install
