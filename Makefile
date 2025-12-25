SHELL := /bin/bash
SCRIPTS_DIR := $(CURDIR)/scripts

MODULE_NAME = goxel
REPO_PATH = $(shell git rev-parse --show-toplevel || pwd)
REPO_NAME = $(shell basename $$REPO_PATH)
GIT_SHA = $(shell git rev-parse --short HEAD)
BUILD_DATE = $(shell date +%Y-%m-%d)
BUILD_TIME = $(shell date +%H:%M:%S)

TAG_DATE ?= $(shell git log -1 --format=%cd --date=format:'%Y%m%d_%H%M%S')
TAG = $(shell git describe --tags --abbrev=0)

all: test build

help:  ## Prints the help/usage docs.
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST) | sort

nuke:  ## Resets the project to its initial state.
	git clean -ffdqx

clean:  ## Removes build/test outputs.
	rm -rf bin *.test
	go clean

update-deps:  ## Tidies up the go module.
	go clean -modcache && rm -rf vendor go.sum && go get -u ./...&& go mod tidy && go mod vendor	

### TEST commands ####
# gotest.tools/gotestsum@latest
test:  ## Runs short tests.
	go test -short -v ./pkg/...

test-report: ## Runs ALL tests with junit report output
	go install gotest.tools/gotestsum@latest || true && \
    mkdir -p tmp && gotestsum --junitfile tmp/report.xml --format testname ./pkg/...

.PHONY: integration-test
	go test -v ./pkg/...

# github.com/golangci/golangci-lint/cmd/golangci-lint@latest
lint:  ## Run static code analysis
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest || true
	golangci-lint run ./pkg/...

lint-report: ## Run golangci-lint report
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest || true
	mkdir -p tmp && golangci-lint run --issues-exit-code=0 --output.text.print-issued-lines=false --output.code-climate.path=tmp/gl-code-quality-report.json

vet:  ## Runs Golang's static code analysis
	go vet ./pkg/...

# golang.org/x/vuln/cmd/govulncheck@latest 
vulnerability:  ## Runs the vulnerability check.
	go install golang.org/x/vuln/cmd/govulncheck@latest || true && \
	govulncheck ./pkg/...

vulnerability-report:  ## Runs the vulnerability check.
	go install golang.org/x/vuln/cmd/govulncheck@latest || true && \
	mkdir -p tmp && govulncheck -json ./pkg/... > tmp/go-vuln-report.json


build-ctl: ## Builds the goxel ui
	mkdir -p bin
	CGO_ENABLED=0 go build \
		-trimpath \
		-ldflags "-s -w -X 'main.GitSHA=$(GIT_SHA)'" \
		-o bin/goxel \
		cmd/main.go
