all: i

.PHONY: i wsl wslc wsl7 wslc7 clean ls b igo7 win win7 iscoop igo imake icode igit

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
BUILD := build -ldflags="-s -w"


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

b:
	go $(BUILD)&& \
	$(CP) $(call NORM,$(PROGRAM_NAME)$(GOEXE)) $(TARGET)

clean:
	$(RM) $(TARGET)

ls:
	$(LS) $(TARGET)

wsl:
	GOOS=windows \
	CGO_ENABLED=0 \
	go $(BUILD)&& \
	cp "$(PROGRAM_NAME).exe" "$(WSLPATH)/bin/"

wslc:
	GOOS=windows \
	CGO_ENABLED=1 \
	CC=x86_64-w64-mingw32-gcc \
	go $(BUILD)&& \
	cp "$(PROGRAM_NAME).exe" "$(WSLPATH)/bin/"

wsl7:
	GOOS=windows \
	GOARCH=386 \
	CGO_ENABLED=0 \
	$(WIN7) $(BUILD) \
	-o "$(PROGRAM_NAME)-win7.exe"&& \
	cp "$(PROGRAM_NAME)-win7.exe" "$(WSLPATH)/bin/"

wslc7:
	GOOS=windows \
	GOARCH=386 \
	CGO_ENABLED=1 \
	CC=i686-w64-mingw32-gcc \
	$(WIN7) $(BUILD) \
	-o "$(PROGRAM_NAME)-win7.exe"&& \
	cp "$(PROGRAM_NAME)-win7.exe" "$(WSLPATH)/bin/"

win7:
	GOOS=windows GOARCH=386 $(WIN7) $(BUILD) -o "$(PROGRAM_NAME)-win7.exe"

win:
	GOOS=windows go $(BUILD)

linux:
	GOOS=linux go $(BUILD)

iwin:
	GOOS=windows go $(INSTALL)

igo7:
	go install golang.org/dl/$(WIN7)@latest&& \
	$(WIN7) download

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

igit:
ifeq ($(GOEXE),.exe)
	scoop install git
else
	sudo apt install git
endif

iCGOalt:
	su - -c "apt-get update && \
		apt-get install -y \
		libX11-devel \
		libXtst-devel \
		libXext-devel \
		pkg-config \
		gcc \
		make \
		git"

iCGOdeb:
	sudo apt-get update
	sudo apt-get install -y \
		build-essential \
		libx11-dev \
		libxtst-dev \
		libxext-dev \
		x11proto-record-dev \
		pkg-config \
		gcc \
		libc6-dev

p10:
	@echo "=== Сборка в ALT Linux p10 (GLIBC 2.32) ==="
	@echo "Монтируем корень проекта: $(CURDIR)/../.."
	@docker run --rm -v $(CURDIR)/../..:/app -w /app \
		alt:p10 \
		sh -c "apt-get update && \
		       apt-get install -y gcc golang libX11-devel libXtst-devel ca-certificates && \
		       CGO_ENABLED=1 GOOS=linux go build -ldflags='-s -w' -o EnRuP10 ./cmd/EnRu && \
		       echo '✅ Сборка завершена' && \
		       echo '' && \
		       echo 'Зависимости:' && \
		       ldd EnRuP10 && \
		       echo '' && \
		       echo 'Версии GLIBC:' && \
		       strings EnRuP10 | grep 'GLIBC_' | sort -u"
	@echo "✓ Бинарник: $(CURDIR)/../.. /EnRuP10"