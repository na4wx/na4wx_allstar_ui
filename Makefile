BINARY := hamvoip-gui
PKG := ./cmd/hamvoip-gui

.PHONY: build run test clean pi pi64 fmt

# CGO_ENABLED=0 everywhere: nothing in this project needs cgo, and
# disabling it produces a fully static binary with no dependency on a
# working C toolchain on the build machine — which matters when
# building natively on a Pi, where system C headers can be mismatched
# (e.g. a 64-bit kernel under a 32-bit userland).

build:
	CGO_ENABLED=0 go build -o bin/$(BINARY) $(PKG)

run: build
	./bin/$(BINARY)

test:
	go test ./...

fmt:
	gofmt -l -w .

# Raspberry Pi Zero / 1 / 2 (32-bit, armv6 covers all of them incl. Zero)
pi:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build -o bin/$(BINARY)-armv6 $(PKG)

# Raspberry Pi 3 / 4 running a 64-bit OS
pi64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/$(BINARY)-arm64 $(PKG)

clean:
	rm -rf bin
