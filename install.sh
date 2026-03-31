#!/usr/bin/env bash
set -euo pipefail

REPO="jasonraimondi/plan-bender"
BINARY="plan-bender"
AGENT_BINARY="plan-bender-agent"
INSTALL_DIR="${INSTALL_DIR:-~/.local/bin}"

for cmd in curl tar; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Required command not found: $cmd" >&2; exit 1; }
done

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
  x86_64)        ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)             echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Get version (override with VERSION env var)
if [ -z "${VERSION:-}" ]; then
  API_RESPONSE="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")"
  if command -v jq >/dev/null 2>&1; then
    VERSION="$(echo "$API_RESPONSE" | jq -r '.tag_name' | sed 's/^v//')"
  else
    VERSION="$(echo "$API_RESPONSE" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')"
  fi
  if [ -z "$VERSION" ]; then
    echo "Failed to fetch latest version" >&2
    exit 1
  fi
fi
echo "Installing ${BINARY} v${VERSION} (${OS}/${ARCH})"

# Download and extract
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

ASSET="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ASSET}"

curl -fsSL "$URL" -o "${TMP}/${ASSET}"
tar xzf "${TMP}/${ASSET}" -C "$TMP" "$BINARY" "$AGENT_BINARY"
chmod +x "${TMP}/${BINARY}" "${TMP}/${AGENT_BINARY}"

# Ensure install dir exists; use sudo only if needed
if [ ! -d "$INSTALL_DIR" ]; then
  mkdir -p "$INSTALL_DIR" 2>/dev/null || sudo mkdir -p "$INSTALL_DIR"
fi
SUDO=""
[ -w "$INSTALL_DIR" ] || SUDO="sudo"

$SUDO mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
$SUDO mv "${TMP}/${AGENT_BINARY}" "${INSTALL_DIR}/${AGENT_BINARY}"
$SUDO ln -sf "${INSTALL_DIR}/${BINARY}" "${INSTALL_DIR}/pb"
$SUDO ln -sf "${INSTALL_DIR}/${AGENT_BINARY}" "${INSTALL_DIR}/pba"

echo "Installed pb v${VERSION} to ${INSTALL_DIR}"

case ":$PATH:" in
  *":${INSTALL_DIR}:"*) ;;
  *) echo "Warning: ${INSTALL_DIR} is not on your PATH. Add it with:"
     echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
     ;;
esac

echo "Next: cd into your project and run pb setup"
