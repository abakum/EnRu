PROGRAM_NAME = EnRu
GOPATH = $(shell go env GOPATH)
BIN_PATH = $(GOPATH)/bin
GOEXE = $(shell go env GOEXE)

INSTALL := install -ldflags="-s -w"
BUILD   := build -ldflags="-s -w"

.PHONY: all windows c1c2 linux darwin clean install windows-install linux-install darwin-install

all: windows linux

windows:
	GOOS=windows CGO_ENABLED=0 go $(BUILD) -o "$(PROGRAM_NAME).exe" ./cmd/EnRu/

c1c2:
	GOOS=windows CGO_ENABLED=0 go $(BUILD) -o c1c2.exe ./cmd/c1c2/

linux:
	GOOS=linux CGO_ENABLED=1 go $(BUILD) -o "$(PROGRAM_NAME)" ./cmd/EnRu/

# darwin нужно собирать НА macOS: cgo требует фреймворков Apple и
# не кросс-компилируется с Linux/Windows.
darwin:
	GOOS=darwin CGO_ENABLED=1 go $(BUILD) -o "$(PROGRAM_NAME)-darwin" ./cmd/EnRu/

install: windows-install linux-install

windows-install:
	GOOS=windows CGO_ENABLED=0 go $(INSTALL) ./cmd/EnRu/

linux-install:
	GOOS=linux CGO_ENABLED=1 go $(INSTALL) ./cmd/EnRu/

darwin-install:
	GOOS=darwin CGO_ENABLED=1 go $(INSTALL) ./cmd/EnRu/

clean:
	rm -f "$(PROGRAM_NAME)" "$(PROGRAM_NAME).exe" "$(PROGRAM_NAME)-darwin" c1c2.exe
