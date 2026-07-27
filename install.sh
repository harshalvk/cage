#!/usr/bin/env bash
set -euo pipefail

REPO="harshalvk/cage"
BINARY="cage"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac

LATEST=$(curl -sf "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
URL="https://github.com/$REPO/releases/download/$LATEST/${BINARY}_${OS}_${ARCH}.tar.gz"

echo "Installing cage $LATEST for $OS/$ARCH..."
curl -sL "$URL" | tar xz -C /tmp
sudo mv "/tmp/$BINARY" /usr/local/bin/$BINARY
echo "Installed. Run 'cage --version' to confirm."