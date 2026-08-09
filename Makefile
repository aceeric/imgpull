CMD_VERSION := v1.14.0
DATETIME    := $(shell date -u +%Y-%m-%dT%T.%2NZ)
ROOT        := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

.PHONY : all
all:
	@echo Run 'make help' to see a list of available targets

.PHONY: test
test:
	go test $(ROOT)/pkg/imgpull $(ROOT)/internal/... -v --cover

.PHONY: coverprof
coverprof:
	go test $(ROOT)/pkg/imgpull $(ROOT)/internal/... -coverprofile=$(ROOT)/prof.out
	go tool cover -html=$(ROOT)/prof.out

.PHONY: update-modules
update-modules:
	go get -u ./...
	rm go.sum
	sed -i '/\/\/ indirect/d' go.mod
	go mod tidy

.PHONY: imgpull
imgpull:
	CGO_ENABLED=0 go build -ldflags "-X 'main.buildVer=$(CMD_VERSION)' -X 'main.buildDtm=$(DATETIME)'"\
	 -a -o $(ROOT)/bin/imgpull $(ROOT)/cmd/imgpull/*.go

.PHONY: install
install:
	cp bin/imgpull /usr/bin/imgpull

.PHONY : help
help:
	@echo "$$HELPTEXT"

export HELPTEXT
define HELPTEXT
This make file provides the following targets:

test            Runs the unit tests

coverprof       Runs the test coverage profile report and displays it in a local
                browser window.

update-modules  Runs 'go get -u' and 'go mod tidy'

imgpull         Builds the CLI. After building then: 'bin/imgpull --help'.

install         Copies bin/imgpull to /usr/bin/imgpull (must run as root)
endef
