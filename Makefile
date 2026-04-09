THIS_DIR := $(dir $(lastword $(MAKEFILE_LIST)))

include ./.bingo/Variables.mk

SOURCES = $(shell find $(THIS_DIR) -name '*.go' -o -name 'go.mod' -o -name 'go.sum')

BINARY_NAME = weather-telemetry
WEATHER_TELEMETRY = $(THIS_DIR)$(BINARY_NAME)
$(WEATHER_TELEMETRY): $(SOURCES) $(MAKEFILE_LIST)
	cd $(THIS_DIR) && go build -o $(BINARY_NAME) ./cmd

test:
	go test ./...

tidy:
	go mod tidy

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run -v ./...
.PHONY: lint

lint-fix: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --fix -v ./...
.PHONY: lint-fix