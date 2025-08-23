all: install
install:
	go install -ldflags="-s -w -H windowsgui"
wsl:
	GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go install -ldflags="-s -w -H windowsgui"

.PHONY: install wsl
