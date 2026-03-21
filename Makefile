.PHONY: build install install-system lint test clean

build:
	go build -o bin/setupmac ./cmd/setupmac

install:
	go install ./cmd/setupmac

install-system:
	go build -o /usr/local/bin/setupmac ./cmd/setupmac

lint:
	go vet ./...

test:
	go test ./...

clean:
	rm -rf bin/
