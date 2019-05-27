PROJ=wxgo
ORG_PATH=github.com/fishioon
REPO_PATH=$(ORG_PATH)/$(PROJ)
VERSION=`git rev-parse --short HEAD`
BUILD=`date +%FT%T%z`

LDFLAGS=-ldflags "-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD}"

build:
	go build ${LDFLAGS} -o wxgo

.PHONY:  clean install
