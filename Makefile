.DEFAULT_GOAL := build
MAKEFLAGS += --no-print-directory

GOPATH=$(shell go env GOPATH)
GOBIN=$(GOPATH)/bin

GITSHA=$(shell git log -1 --pretty=format:%h)
GITTAG=$(shell git tag --points-at HEAD -n1 --format="%(refname:strip=2)")

DOCKERIMAGE=ghcr.io/cloudeteer/m365-exporter
DEVIMAGE=$(DOCKERIMAGE):$(GITSHA)
RELEASEIMAGE=$(DOCKERIMAGE):$(GITTAG)

##
# Console Colors
##
GREEN  := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
WHITE  := $(shell tput -Txterm setaf 7)
CYAN   := $(shell tput -Txterm setaf 6)
RESET  := $(shell tput -Txterm sgr0)

##
# Targets
##
.PHONY: help
help: ## show this help.
	@echo 'Usage:'
	@echo '  ${GREEN}make${RESET} ${YELLOW}<target>${RESET}'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} { \
		if (/^[a-zA-Z_-]+:.*?##.*$$/) {printf "  ${GREEN}%-21s${YELLOW}%s${RESET}\n", $$1, $$2} \
		else if (/^## .*$$/) {printf "  ${CYAN}%s${RESET}\n", substr($$1,4)} \
		}' $(MAKEFILE_LIST)

.PHONY: build
build: m365-exporter ## Builds the app binary only

.PHONY: run
run: m365-exporter run-binary ## Builds the binary and starts it on your local machine

.PHONY: all
all: m365-exporter

.PHONY: test
test: unit-tests ## Runs all tests

.PHONY: m365-exporter
m365-exporter:
	goreleaser build --snapshot --single-target --clean

.PHONY: unit-tests
unit-tests:
	@go get -t ./... && go test -v ./...

.PHONY: run-binary
run-binary:
	./dist/*/m365-exporter

.PHONY: clean
clean: ## Removes binary artifacts
	@rm --force bin/* --recursive dist/

.PHONY: lint
lint: ## lint source code
	golangci-lint run

.PHONY: fmt
fmt: ## Format source code
	go fmt ./...
	-go run mvdan.cc/gofumpt@latest -l -w .
	-go run golang.org/x/tools/cmd/goimports@latest -l -w .
	-go run github.com/bombsimon/wsl/v4/cmd...@latest -strict-append -test=true -fix ./...
	-go run github.com/catenacyber/perfsprint@latest -fix ./...
	-go run github.com/tetafro/godot/cmd/godot@latest -w .
