#!/usr/bin/env bash
set -euo pipefail

REPO="jasonraimondi/plan-bender"
BINARY="plan-bender"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)      echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect arch
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)       ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)             echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Get latest version
VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')"
if [ -z "$VERSION" ]; then
  echo "Failed to fetch latest version" >&2
  exit 1
fi
echo "Installing ${BINARY} v${VERSION} (${OS}/${ARCH})"

# Download and extract
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

ASSET="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ASSET}"

curl -fsSL "$URL" -o "${TMP}/${ASSET}"
tar xzf "${TMP}/${ASSET}" -C "$TMP" "$BINARY"
chmod +x "${TMP}/${BINARY}"

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  ln -sf "${INSTALL_DIR}/${BINARY}" "${INSTALL_DIR}/pb"
else
  sudo mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  sudo ln -sf "${INSTALL_DIR}/${BINARY}" "${INSTALL_DIR}/pb"
fi

echo "Installed ${BINARY} v${VERSION} to ${INSTALL_DIR}/${BINARY}"
echo "Symlinked ${INSTALL_DIR}/pb -> ${INSTALL_DIR}/${BINARY}"
