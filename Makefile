all: test build

lint:
	go mod tidy
	go mod vendor

build: lint
	go build -o bin/surbot main.go

test: lint
	go fmt ./...
	go vet -v ./...
	go test -race -v ./...

run:
	go run main.go

clean:
	rm -rf bin/
