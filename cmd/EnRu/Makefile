all: install

.PHONY: install wsl wsl7 clean list build

# Project configuration
PROGRAM_NAME = $(notdir $(CURDIR))
GOEXE = $(shell go env GOEXE)
BIN_PATH = $(shell go env GOPATH)/bin
TARGET = $(BIN_PATH)/$(PROGRAM_NAME)$(GOEXE)
WINDOWS_GOPATH = $(shell cmd.exe /c "echo %GOPATH%" 2>/dev/null | tr -d '\r\n')
WSLPATH = $(shell wslpath -u "$(WINDOWS_GOPATH)")

# Installation settings
INSTALL = install -ldflags="-s -w"
BUILD = build -ldflags="-s -w -extldflags=-static"

# Path formatting for different OS
ifeq ($(GOEXE),.exe)
    TARGET = $(subst /,\,$(BIN_PATH)/$(PROGRAM_NAME)$(GOEXE))
endif

install:
	@echo "Installing $(PROGRAM_NAME)$(GOEXE) to $(BIN_PATH)..."
	go $(INSTALL)

clean:
	@echo "Removing $(PROGRAM_NAME)$(GOEXE)..."
ifeq ($(GOEXE),.exe)
	@-if exist "$(TARGET)" del /Q "$(TARGET)" 2>nul || true
else
	@-rm -f "$(TARGET)" 2>/dev/null || true
endif

list:
	@echo "Project: $(PROGRAM_NAME)"
	@echo "Binary: $(TARGET)"
ifeq ($(GOEXE),.exe)
	@-if exist "$(TARGET)" (dir "$(TARGET)") else (echo File not found)
else
	@-if [ -f "$(TARGET)" ]; then ls -l "$(TARGET)"; else echo "File not found"; fi
endif

wsl:
	GOOS=windows \
	GOARCH=amd64 \
	CC=x86_64-w64-mingw32-gcc \
	go $(BUILD)
	cp $(PROGRAM_NAME).exe "$(WSLPATH)/bin/"

wsl7:
# 	goenv install 1.20.7
	GOOS=windows \
	GOARCH=386 \
	CC=i686-w64-mingw32-gcc \
	$(HOME)/.goenv/versions/1.20.7/bin/go $(BUILD) \
	-o $(PROGRAM_NAME)-win7.exe
	cp $(PROGRAM_NAME)-win7.exe "$(WSLPATH)/bin/"

build:
	go $(BUILD)
