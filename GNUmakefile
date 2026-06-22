.PHONY: default build test lint fmt tidy install

default: build

build:
	go build ./...

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

install:
	go install .
