all: install

.PHONY: install wsl wsl7 clean list build

# Project configuration
PROGRAM_NAME = $(notdir $(CURDIR))
GOEXE = $(shell go env GOEXE)
GOPATH = $(shell go env GOPATH)
BIN_PATH = $(GOPATH)/bin
TARGET = "$(BIN_PATH)/$(PROGRAM_NAME)$(GOEXE)"
WINDOWS_GOPATH = $(shell cmd.exe /c "echo %GOPATH%" 2>/dev/null | tr -d '\r\n')
WSLPATH = $(shell wslpath -u "$(WINDOWS_GOPATH)")
WIN7 := go1.21.4

# Installation settings
INSTALL = install -ldflags="-s -w"
BUILD = build -ldflags="-s -w -extldflags=-static"

# Path formatting for different OS
ifeq ($(GOEXE),.exe)
    TARGET = "$(subst /,\,$(BIN_PATH)/$(PROGRAM_NAME)$(GOEXE))"
    MKDIR  := mkdir
    RMDIR  := rmdir /s /q
    CP     := copy
    RM     := del /q
    PATH = "$(subst /,\,$1)"
else
    MKDIR  := mkdir -p
    RMDIR  := rm -rf
    CP     := cp
    RM     := rm -f
    PATH = "$1"
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
	GOOS=windows \
	GOARCH=386 \
	CC=i686-w64-mingw32-gcc \
	$(WIN7) $(BUILD) \
	-o $(PROGRAM_NAME)-win7.exe
	cp $(PROGRAM_NAME)-win7.exe "$(WSLPATH)/bin/"

igo7:
	go install golang.org/dl/go1.20.7@latest
	go1.20.7 download

igo4:
	go install golang.org/dl/go1.21.4@latest
	go1.21.4 download

go7:
ifeq ($(GOEXE),.exe)
	set GOOS=windows&& \
	set GOARCH=386&& \
	$(WIN7) $(BUILD) -o $(PROGRAM_NAME)-win7.exe&& \
	copy $(PROGRAM_NAME)-win7.exe "$(GOPATH)\bin\"
else
	GOOS=windows GOARCH=386 go1.20.7 $(BUILD) -o $(PROGRAM_NAME)-win7.exe
endif

build:
	go $(BUILD)&& \
	$(CP) $(call PATH,$(PROGRAM_NAME)$(GOEXE)) $(TARGET)

scoop:
	powershell -Command "Set-ExecutionPolicy RemoteSigned -Scope CurrentUser"
    powershell -Command "iex (new-object net.webclient).downloadstring('https://get.scoop.sh')"

igo:
ifeq ($(GOEXE),.exe)
	scoop install go
else
	sudo apt install golang-go
endif

make:
ifeq ($(GOEXE),.exe)
	scoop install make
else
	sudo apt install make
endif

vscode:
	scoop bucket add extras
	scoop install vscode