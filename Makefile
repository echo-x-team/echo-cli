GO ?= go
BIN ?= bin/echo-cli
CMD ?= ./cmd/echo-cli
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
INSTALL ?= install
TARGET ?= echo-cli

.PHONY: build
build:
	@mkdir -p $(dir $(BIN))
	$(GO) build -o $(BIN) $(CMD)

.PHONY: install
install: build
	$(INSTALL) -d $(DESTDIR)$(BINDIR)
	$(INSTALL) $(BIN) $(DESTDIR)$(BINDIR)/$(TARGET)
