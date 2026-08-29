GO ?= go
APP := singbox-switch
CMD := ./cmd/singbox-switch
DIST := dist

HOST_OS := $(shell $(GO) env GOOS)
HOST_ARCH := $(shell $(GO) env GOARCH)

.PHONY: all build build-all build-macos-amd64 build-macos-arm64 build-linux-amd64 build-linux-arm64 build-windows-amd64 test vet clean

all: build

build: | $(DIST)
	GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) $(GO) build -o $(DIST)/$(APP)-$(HOST_OS)-$(HOST_ARCH) $(CMD)

$(DIST):
	mkdir -p $@

build-macos-amd64: | $(DIST)
	GOOS=darwin GOARCH=amd64 $(GO) build -o $(DIST)/$(APP)-macos-amd64 $(CMD)

build-macos-arm64: | $(DIST)
	GOOS=darwin GOARCH=arm64 $(GO) build -o $(DIST)/$(APP)-macos-arm64 $(CMD)

build-linux-amd64: | $(DIST)
	GOOS=linux GOARCH=amd64 $(GO) build -o $(DIST)/$(APP)-linux-amd64 $(CMD)

build-linux-arm64: | $(DIST)
	GOOS=linux GOARCH=arm64 $(GO) build -o $(DIST)/$(APP)-linux-arm64 $(CMD)

build-windows-amd64: | $(DIST)
	GOOS=windows GOARCH=amd64 $(GO) build -o $(DIST)/$(APP)-windows-amd64.exe $(CMD)

build-all: build-macos-amd64 build-macos-arm64 build-linux-amd64 build-linux-arm64 build-windows-amd64

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

clean:
	rm -f $(DIST)/$(APP)-*
