PROGRAM_NAME = EnRu
GOPATH = $(shell go env GOPATH)
BIN_PATH = $(GOPATH)/bin
GOEXE = $(shell go env GOEXE)

INSTALL := install -ldflags="-s -w"
BUILD   := build -ldflags="-s -w"

.PHONY: all windows linux clean install windows-install linux-install

all: windows linux

windows:
	GOOS=windows CGO_ENABLED=0 go $(BUILD) -o "$(PROGRAM_NAME).exe" ./cmd/windows/EnRu/

linux:
	GOOS=linux CGO_ENABLED=0 go $(BUILD) -o "$(PROGRAM_NAME)" ./cmd/linux/EnRu/

install: windows-install linux-install

windows-install:
	GOOS=windows CGO_ENABLED=0 go $(INSTALL) ./cmd/windows/EnRu/

linux-install:
	GOOS=linux CGO_ENABLED=0 go $(INSTALL) ./cmd/linux/EnRu/

clean:
	rm -f "$(PROGRAM_NAME)" "$(PROGRAM_NAME).exe"