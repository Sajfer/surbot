.PHONY: all lint build docker test run clean

all: lint test build

lint:
	go fmt ./...
	go mod tidy
	golangci-lint run ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/surbot .

docker:
	docker build -t surbot -f build/package/Dockerfile .

test:
	go vet -v ./...
	go test -race -cover -v ./...

run:
	go run main.go

clean:
	rm -rf bin/
