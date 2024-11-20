.PHONY: all install-deps lint build test run clean

all: install-deps lint test build

install-deps:
	go install github.com/mgechev/revive@latest

lint:
	go fmt ./...
	go mod tidy
	go mod vendor
	revive -exclude vendor/... -formatter friendly -config .revive.toml ./...

build:
	go build -o bin/surbot main.go

docker:
	docker build -t surbot -f build/package/Dockerfile .
test:
	go vet -v ./...
	go test -race -cover -v ./...

run:
	go run main.go

clean:
	rm -rf bin/
