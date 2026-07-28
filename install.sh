#!/usr/bin/env bash
set -euo pipefail

# Cage CLI installer — downloads the latest release for your platform,
# verifies it against the published checksums, then installs it.

REPO="harshalvk/cage"
BINARY="cage"
INSTALL_DIR="/usr/local/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Error: unsupported architecture '$ARCH'" >&2
    exit 1
    ;;
esac

case "$OS" in
  linux|darwin) ;;
  *)
    echo "Error: unsupported OS '$OS'. On Windows, use Scoop instead:" >&2
    echo "  scoop bucket add cage https://github.com/harshalvk/scoop-cage" >&2
    echo "  scoop install cage" >&2
    exit 1
    ;;
esac

echo "→ Looking up the latest release..."
LATEST=$(curl -sf "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
if [ -z "$LATEST" ]; then
  echo "Error: could not determine the latest release. Check https://github.com/$REPO/releases" >&2
  exit 1
fi

ARCHIVE="${BINARY}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/$REPO/releases/download/$LATEST"

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

echo "→ Downloading $ARCHIVE ($LATEST)..."
curl -sfL "$BASE_URL/$ARCHIVE" -o "$WORKDIR/$ARCHIVE"

echo "→ Downloading checksums.txt..."
curl -sfL "$BASE_URL/checksums.txt" -o "$WORKDIR/checksums.txt"

echo "→ Verifying checksum..."
cd "$WORKDIR"
EXPECTED=$(grep " $ARCHIVE\$" checksums.txt | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
  echo "Error: no checksum entry found for $ARCHIVE — refusing to install an unverified binary." >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$ARCHIVE" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "$ARCHIVE" | awk '{print $1}')
else
  echo "Error: neither sha256sum nor shasum is available — cannot verify the download." >&2
  exit 1
fi

if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "Error: checksum mismatch!" >&2
  echo "  expected: $EXPECTED" >&2
  echo "  actual:   $ACTUAL" >&2
  echo "The downloaded file does not match the published checksum. Aborting — do not trust this binary." >&2
  exit 1
fi
echo "  checksum OK"

echo "→ Installing to $INSTALL_DIR..."
tar xzf "$ARCHIVE"

if [ -w "$INSTALL_DIR" ]; then
  mv "$BINARY" "$INSTALL_DIR/$BINARY"
else
  sudo mv "$BINARY" "$INSTALL_DIR/$BINARY"
fi

echo "→ Installed $($INSTALL_DIR/$BINARY --version 2>/dev/null || echo "$LATEST")"
echo "Run '$BINARY --help' to get started."