PREFIX ?= $(HOME)/.local
.PHONY: build check test install
build:
	mkdir -p bin
	go build -o bin/reqscope ./cmd/reqscope
check:
	go vet ./...
	go build ./...
test:
	go test -race ./...
install: build
	mkdir -p "$(DESTDIR)$(PREFIX)/bin"
	install -m 755 bin/reqscope "$(DESTDIR)$(PREFIX)/bin/reqscope"
