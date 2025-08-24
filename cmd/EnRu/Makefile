all: install

.PHONY: install wsl clean list build

# Project configuration
PROGRAM_NAME = $(notdir $(CURDIR))
GOEXE = $(shell go env GOEXE)
BIN_PATH = $(shell go env GOPATH)/bin
TARGET = $(BIN_PATH)/$(PROGRAM_NAME)$(GOEXE)

# Installation settings
INSTALL_CMD = go install -ldflags="-s -w"

# Path formatting for different OS
ifeq ($(GOEXE),.exe)
    TARGET = $(subst /,\,$(BIN_PATH)/$(PROGRAM_NAME)$(GOEXE))
endif

install:
	@echo "Installing $(PROGRAM_NAME)$(GOEXE) to $(BIN_PATH)..."
	$(INSTALL_CMD)

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
	GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -ldflags="-s -w"
build:
	go build -ldflags="-s -w"
386:
	set GOOS=windows
	set GOARCH=386
	go build -ldflags="-s -w"
