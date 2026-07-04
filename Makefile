# Makefile for gozik-spotify
# Standard GNU-style targets: all, build, install, install-user, uninstall, uninstall-user, clean
#
# Typical workflow:
#   make && sudo make install
#   sudo systemctl enable --now gozik-spotify
#
# User-level install (no sudo):
#   make && make install-user
#   systemctl --user enable --now gozik-spotify

# Installation prefix (override with: make PREFIX=/opt)
PREFIX ?= /usr/local
BINDIR  ?= $(PREFIX)/bin
LIBDIR  ?= $(PREFIX)/lib
SYSTEMD_SYSTEM_UNIT_DIR ?= /etc/systemd/system

# User-level install paths (no sudo required)
USER_PREFIX       ?= $(HOME)/.local
USER_BINDIR       ?= $(USER_PREFIX)/bin
USER_LIBDIR       ?= $(USER_PREFIX)/lib
USER_INSTALL_DIR  ?= $(USER_LIBDIR)/$(BINARY_NAME)
USER_INSTALL_WRAPPER ?= $(USER_BINDIR)/$(BINARY_NAME)
SYSTEMD_USER_UNIT_DIR ?= $(HOME)/.config/systemd/user

BINARY_NAME := gozik-spotify
BUILDDIR    := build
BUNDLE_BINARY := $(BUILDDIR)/$(BINARY_NAME)
YTDLP_URL   ?= https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp
YTDLP_BINARY := $(BUILDDIR)/yt-dlp
INSTALL_DIR := $(LIBDIR)/$(BINARY_NAME)
INSTALL_WRAPPER := $(BINDIR)/$(BINARY_NAME)

GOFLAGS ?=
LDFLAGS ?=
CURL    ?= curl

.PHONY: all build install install-user uninstall uninstall-user clean distclean

# -----------------------------------------------------------------------------
# Default target
# -----------------------------------------------------------------------------
all: build

# -----------------------------------------------------------------------------
# Build the Go binary
# -----------------------------------------------------------------------------
build: $(BUNDLE_BINARY) $(YTDLP_BINARY)

$(BUNDLE_BINARY):
	@echo "==> Building $(BINARY_NAME) ..."
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUNDLE_BINARY) ./cmd/gozik-spotify
	@echo "==> Built $(BUNDLE_BINARY)"

$(YTDLP_BINARY):
	@echo "==> Downloading yt-dlp into $(BUILDDIR) ..."
	@mkdir -p $(BUILDDIR)
	$(CURL) -fsSL -o $(YTDLP_BINARY) $(YTDLP_URL)
	chmod +x $(YTDLP_BINARY)
	@echo "==> yt-dlp ready at $(YTDLP_BINARY)"

# -----------------------------------------------------------------------------
# Install binary + systemd system service unit
# -----------------------------------------------------------------------------
install:
	@test -f $(BUNDLE_BINARY) || { echo "Binary not found: $(BUNDLE_BINARY). Run 'make' first."; exit 1; }
	@echo "==> Installing application bundle to $(DESTDIR)$(INSTALL_DIR) ..."
	install -d $(DESTDIR)$(INSTALL_DIR)
	install -m 0755 $(BUNDLE_BINARY) $(DESTDIR)$(INSTALL_DIR)/$(BINARY_NAME)
	install -m 0755 $(YTDLP_BINARY) $(DESTDIR)$(INSTALL_DIR)/yt-dlp
	@echo "==> Installing wrapper script to $(DESTDIR)$(INSTALL_WRAPPER) ..."
	install -d $(DESTDIR)$(BINDIR)
	@printf '%s\n' \
		'#!/bin/sh' \
		'export PATH="$(INSTALL_DIR):$$PATH"' \
		'exec "$(INSTALL_DIR)/$(BINARY_NAME)" "$$@"' \
		> $(DESTDIR)$(INSTALL_WRAPPER)
	chmod 0755 $(DESTDIR)$(INSTALL_WRAPPER)
	@mkdir -p $(DESTDIR)$(SYSTEMD_SYSTEM_UNIT_DIR)
	@printf '%s\n' \
		'[Unit]' \
		'Description=gozik Spotify gRPC plugin server' \
		'After=network.target' \
		'' \
		'[Service]' \
		'Type=simple' \
		'Environment=GOZIK_SPOTIFY_PORT=50054' \
		'Environment=GOZIK_SPOTIFY_WEBUI_PORT=50055' \
		'ExecStart=$(INSTALL_WRAPPER)' \
		'Restart=on-failure' \
		'RestartSec=5' \
		'StandardOutput=journal' \
		'StandardError=journal' \
		'' \
		'[Install]' \
		'WantedBy=multi-user.target' \
		> $(DESTDIR)$(SYSTEMD_SYSTEM_UNIT_DIR)/$(BINARY_NAME).service
	@chmod 644 $(DESTDIR)$(SYSTEMD_SYSTEM_UNIT_DIR)/$(BINARY_NAME).service
	@echo ""
	@echo "==> $(BINARY_NAME) installed to $(DESTDIR)$(INSTALL_DIR)"
	@echo "==> Wrapper script installed to $(DESTDIR)$(INSTALL_WRAPPER)"
	@echo "==> systemd unit installed to $(DESTDIR)$(SYSTEMD_SYSTEM_UNIT_DIR)/$(BINARY_NAME).service"
	@echo ""
	@echo "    Start now : systemctl daemon-reload --user"
	@echo "                systemctl enable --user --now $(BINARY_NAME).service"
	@echo "    Status    : sudo systemctl status --user $(BINARY_NAME).service"

# -----------------------------------------------------------------------------
# User-level install (no sudo)
# -----------------------------------------------------------------------------
install-user:
	@test -f $(BUNDLE_BINARY) || { echo "Binary not found: $(BUNDLE_BINARY). Run 'make' first."; exit 1; }
	@echo "==> Installing application bundle to $(USER_INSTALL_DIR) ..."
	install -d $(USER_INSTALL_DIR)
	install -m 0755 $(BUNDLE_BINARY) $(USER_INSTALL_DIR)/$(BINARY_NAME)
	install -m 0755 $(YTDLP_BINARY) $(USER_INSTALL_DIR)/yt-dlp
	@echo "==> Installing wrapper script to $(USER_INSTALL_WRAPPER) ..."
	install -d $(USER_BINDIR)
	@printf '%s\n' \
		'#!/bin/sh' \
		'export PATH="$(USER_INSTALL_DIR):$$PATH"' \
		'exec "$(USER_INSTALL_DIR)/$(BINARY_NAME)" "$$@"' \
		> $(USER_INSTALL_WRAPPER)
	chmod 0755 $(USER_INSTALL_WRAPPER)
	@mkdir -p $(SYSTEMD_USER_UNIT_DIR)
	@printf '%s\n' \
		'[Unit]' \
		'Description=gozik Spotify gRPC plugin server' \
		'After=network.target' \
		'' \
		'[Service]' \
		'Type=simple' \
		'Environment=GOZIK_SPOTIFY_PORT=50054' \
		'Environment=GOZIK_SPOTIFY_WEBUI_PORT=50055' \
		'ExecStart=$(USER_INSTALL_WRAPPER)' \
		'Restart=on-failure' \
		'RestartSec=5' \
		'StandardOutput=journal' \
		'StandardError=journal' \
		'PassEnvironment=DISPLAY XAUTHORITY WAYLAND_DISPLAY' \
		'' \
		'[Install]' \
		'WantedBy=default.target' \
		> $(SYSTEMD_USER_UNIT_DIR)/$(BINARY_NAME).service
	@chmod 644 $(SYSTEMD_USER_UNIT_DIR)/$(BINARY_NAME).service
	@echo ""
	@echo "==> $(BINARY_NAME) installed to $(USER_INSTALL_DIR)"
	@echo "==> Wrapper script installed to $(USER_INSTALL_WRAPPER)"
	@echo "==> systemd user unit installed to $(SYSTEMD_USER_UNIT_DIR)/$(BINARY_NAME).service"
	@echo ""
	@echo "    Start now : systemctl --user daemon-reload"
	@echo "                systemctl --user enable --now $(BINARY_NAME).service"
	@echo "    Status    : systemctl --user status $(BINARY_NAME).service"

# -----------------------------------------------------------------------------
# Uninstall
# -----------------------------------------------------------------------------
uninstall:
	rm -rf $(DESTDIR)$(INSTALL_DIR)
	rm -f $(DESTDIR)$(INSTALL_WRAPPER)
	rm -f $(DESTDIR)$(SYSTEMD_SYSTEM_UNIT_DIR)/$(BINARY_NAME).service
	@echo "==> $(BINARY_NAME) uninstalled."
	@echo "    Run: sudo systemctl daemon-reload"

uninstall-user:
	rm -rf $(USER_INSTALL_DIR)
	rm -f $(USER_INSTALL_WRAPPER)
	rm -f $(SYSTEMD_USER_UNIT_DIR)/$(BINARY_NAME).service
	@echo "==> $(BINARY_NAME) user install uninstalled."
	@echo "    Run: systemctl --user daemon-reload"

# -----------------------------------------------------------------------------
# Clean build artifacts
# -----------------------------------------------------------------------------
clean:
	rm -f $(YTDLP_BINARY)
	rm -f $(BUNDLE_BINARY)
	rm -rf $(BUILDDIR)/
	go clean ./...

distclean: clean
