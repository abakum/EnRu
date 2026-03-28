PROGRAM_NAME = EnRu
GOPATH = $(shell go env GOPATH)
BIN_PATH = $(GOPATH)/bin
GOEXE = $(shell go env GOEXE)

INSTALL := install -ldflags="-s -w"
BUILD   := build -ldflags="-s -w"

.PHONY: all windows linux clean install windows-install linux-install

all: windows linux

windows:
	GOOS=windows CGO_ENABLED=0 go $(BUILD) -o "$(PROGRAM_NAME).exe" ./cmd/EnRu/

linux:
	GOOS=linux CGO_ENABLED=1 go $(BUILD) -o "$(PROGRAM_NAME)" ./cmd/EnRu/

install: windows-install linux-install

windows-install:
	GOOS=windows CGO_ENABLED=0 go $(INSTALL) ./cmd/EnRu/

linux-install:
	GOOS=linux CGO_ENABLED=1 go $(INSTALL) ./cmd/EnRu/

clean:
	rm -f "$(PROGRAM_NAME)" "$(PROGRAM_NAME).exe"