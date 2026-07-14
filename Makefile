BINARY := hamvoip-gui
PKG := ./cmd/hamvoip-gui

.PHONY: build run test clean pi pi64 fmt

build:
	go build -o bin/$(BINARY) $(PKG)

run: build
	./bin/$(BINARY)

test:
	go test ./...

fmt:
	gofmt -l -w .

# Raspberry Pi Zero / 1 / 2 (32-bit, armv6 covers all of them incl. Zero)
pi:
	GOOS=linux GOARCH=arm GOARM=6 go build -o bin/$(BINARY)-armv6 $(PKG)

# Raspberry Pi 3 / 4 running a 64-bit OS
pi64:
	GOOS=linux GOARCH=arm64 go build -o bin/$(BINARY)-arm64 $(PKG)

clean:
	rm -rf bin
