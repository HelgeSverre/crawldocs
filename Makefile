BINARY_NAME=crawldocs

.PHONY: build clean install deps run test dev

build:
	go build -o $(BINARY_NAME) .

clean:
	go clean
	rm -f $(BINARY_NAME)

install:
	go install .

deps:
	go mod tidy

run:
	go run .

test:
	go test ./... -v

test-cover:
	go test ./... -cover

dev:
	go run . -help

.DEFAULT_GOAL := build