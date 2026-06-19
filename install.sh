#!/bin/sh
set -e

REPO="chxmxii/3a"
BINARY="3a"
INSTALL_DIR="/usr/local/bin"

# Detect OS and arch.
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Get latest version.
VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
if [ -z "$VERSION" ]; then
  echo "Failed to fetch latest version"
  exit 1
fi

FILENAME="${BINARY}-${OS}-${ARCH}"
URL="https://github.com/$REPO/releases/download/$VERSION/$FILENAME"

echo "Installing $BINARY $VERSION ($OS/$ARCH)..."

# Download.
TMP=$(mktemp)
if ! curl -fsSL "$URL" -o "$TMP"; then
  echo "Download failed: $URL"
  exit 1
fi

chmod +x "$TMP"

# Install.
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP" "$INSTALL_DIR/$BINARY"
else
  sudo mv "$TMP" "$INSTALL_DIR/$BINARY"
fi

echo "Installed $BINARY $VERSION to $INSTALL_DIR/$BINARY"
echo ""
echo "Run: 3a configure"
