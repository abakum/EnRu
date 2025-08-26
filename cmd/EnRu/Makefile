all: i

.PHONY: i wsl wsl7 clean ls b igo7 go7 iscoop igo imake icode igit

# Project configuration
PROGRAM_NAME = $(notdir $(CURDIR))
GOEXE = $(shell go env GOEXE)
GOPATH = $(shell go env GOPATH)
BIN_PATH = $(GOPATH)/bin
WINDOWS_GOPATH = $(shell cmd.exe /c "echo %GOPATH%" 2>/dev/null | tr -d '\r\n')
WSLPATH = $(shell wslpath -u "$(WINDOWS_GOPATH)")
#WIN7 := go1.20.7
WIN7 := go1.21.4

INSTALL := install -ldflags="-s -w"
BUILD := build -ldflags="-s -w -extldflags=-static"


ifeq ($(GOEXE),.exe)
    MKDIR  := mkdir
    RMDIR  := rmdir /s /q
    CP     := copy
    RM     := del /q
	LS     := dir
    NORM = "$(subst /,\,$1)"
else
    MKDIR  := mkdir -p
    RMDIR  := rm -rf
    CP     := cp
    RM     := rm -f
	LS     := ls -l
    NORM = "$1"
endif
TARGET := $(call NORM,$(BIN_PATH)/$(PROGRAM_NAME)$(GOEXE))

i:
	go $(INSTALL)

clean:
	@$(RM) $(TARGET)

ls:
	@$(LS) $(TARGET)

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
	go install golang.org/dl$(WIN7)@latest&& \
	$(WIN7) download

go7:
ifeq ($(GOEXE),.exe)
	set GOOS=windows&& \
	set GOARCH=386&& \
	$(WIN7) $(BUILD) -o $(PROGRAM_NAME)-win7.exe&& \
	copy "$(PROGRAM_NAME)-win7.exe" "$(GOPATH)\bin\"
else
	GOOS=windows GOARCH=386 $(WIN7) $(BUILD) -o "$(PROGRAM_NAME)-win7.exe"
endif

b:
	go $(BUILD)&& \
	$(CP) $(call NORM,$(PROGRAM_NAME)$(GOEXE)) $(TARGET)

iscoop:
	powershell -Command "Set-ExecutionPolicy RemoteSigned -Scope CurrentUser"&& \
    powershell -Command "iex (new-object net.webclient).downloadstring('https://get.scoop.sh')"

igo:
ifeq ($(GOEXE),.exe)
	scoop install go
else
	sudo apt install golang-go
endif

imake:
ifeq ($(GOEXE),.exe)
	scoop install make
else
	sudo apt install make
endif

icode:
	scoop bucket add extras&& \
	scoop install vscode

git:
ifeq ($(GOEXE),.exe)
	scoop install git
else
	sudo apt install git
endif
