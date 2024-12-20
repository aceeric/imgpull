CMD_VERSION := 1.1.0
DATETIME    := $(shell date -u +%Y-%m-%dT%T.%2NZ)
ROOT        := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

.PHONY : all
all:
	@echo Run 'make help' to see a list of available targets

.PHONY: test
test:
	go test $(ROOT)/pkg/imgpull

.PHONY: imgpull
imgpull:
	CGO_ENABLED=0 go build -ldflags "-X 'main.buildVer=$(CMD_VERSION)' -X 'main.buildDtm=$(DATETIME)'"\
	 -a -o $(ROOT)/bin/imgpull $(ROOT)/cmd/imgpull/*.go

.PHONY : help
help:
	@echo "$$HELPTEXT"

export HELPTEXT
define HELPTEXT
This make file provides the following targets:

test          Runs the unit tests

imgpull       Builds the CLI. After building then: 'bin/imgpull --help'.

endef
