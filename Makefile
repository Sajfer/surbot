.PHONY: all lint build test run clean

all: lint test build

lint:
	go fmt ./...
	go mod tidy
	go mod vendor
	go list ./... | grep -v vendor | xargs revive -config .revive.toml -formatter friendly

build:
	go build -o bin/surbot main.go

test:
	go vet -v ./...
	go test -race -v ./...

run:
	go run main.go

clean:
	rm -rf bin/
