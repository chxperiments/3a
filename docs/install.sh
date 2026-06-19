#!/bin/sh
set -e

REPO="chxmxii/a3"
BINARY="a3"
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

# confirm prompts the user for a yes/no answer. It reads from the controlling
# terminal so it works even when this script is piped (curl ... | bash). In a
# non-interactive environment (no tty) it returns false so nothing is installed
# without explicit confirmation.
confirm() {
  if [ ! -e /dev/tty ]; then
    return 1
  fi
  printf "%s [y/N] " "$1" > /dev/tty
  read ans < /dev/tty || return 1
  case "$ans" in
    [Yy] | [Yy][Ee][Ss]) return 0 ;;
    *) return 1 ;;
  esac
}

# A3 needs Steampipe (plus a provider plugin) to query cloud resources. Offer to
# install them now; each action is gated on explicit confirmation.
if ! command -v steampipe >/dev/null 2>&1; then
  echo "Steampipe is not installed. A3 uses it to query cloud resources."
  if confirm "Install Steampipe now?"; then
    echo "Installing Steampipe (you may be prompted for your password)..."
    sudo /bin/sh -c "$(curl -fsSL https://steampipe.io/install/steampipe.sh)"
  else
    echo "Skipping Steampipe. Install it later from https://steampipe.io/downloads"
  fi
fi

if command -v steampipe >/dev/null 2>&1; then
  if confirm "Install the AWS Steampipe plugin?"; then
    steampipe plugin install aws
  fi
  if confirm "Install the OCI Steampipe plugin?"; then
    steampipe plugin install oci
  fi
fi

echo ""
echo "Run: a3 configure"
