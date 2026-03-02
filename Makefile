.PHONY: build test vet clean docker

BINARY := searchmux
GOARCH ?= $(shell go env GOARCH)
GOOS ?= $(shell go env GOOS)

build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="-s -w" -o $(BINARY) ./cmd/searchmux

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)

docker:
	docker build -t searchmux .
