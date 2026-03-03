#!/bin/sh
set -e

BINARY_NAME="${BINARY_NAME:-hai}"
DOWNLOAD_BASE="${DOWNLOAD_BASE:-https://github.com/un-seen/cli-app/releases/latest/download}"
TOKEN_ENV_VAR="${TOKEN_ENV_VAR:-HEDWIGAI_AUTH_TOKEN}"

# Detect OS.
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)
    echo "Error: unsupported operating system: $OS"
    exit 1
    ;;
esac

# Detect architecture.
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Error: unsupported architecture: $ARCH"
    exit 1
    ;;
esac

BINARY="${BINARY_NAME}-${OS}-${ARCH}"
URL="${DOWNLOAD_BASE}/${BINARY}"
CHECKSUM_URL="${DOWNLOAD_BASE}/checksums.txt"

echo "  Detected: ${OS}/${ARCH}"

# Create a temporary directory for the download.
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# Download binary.
echo "  Downloading ${BINARY_NAME}..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "${TMPDIR}/${BINARY}" "$URL"
elif command -v wget >/dev/null 2>&1; then
  wget -q -O "${TMPDIR}/${BINARY}" "$URL"
else
  echo "Error: curl or wget is required"
  exit 1
fi

# Download and verify checksum.
echo "  Verifying checksum..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "${TMPDIR}/checksums.txt" "$CHECKSUM_URL" 2>/dev/null || true
elif command -v wget >/dev/null 2>&1; then
  wget -q -O "${TMPDIR}/checksums.txt" "$CHECKSUM_URL" 2>/dev/null || true
fi

if [ -f "${TMPDIR}/checksums.txt" ]; then
  EXPECTED=$(grep "$BINARY" "${TMPDIR}/checksums.txt" | awk '{print $1}')
  if [ -n "$EXPECTED" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      ACTUAL=$(sha256sum "${TMPDIR}/${BINARY}" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
      ACTUAL=$(shasum -a 256 "${TMPDIR}/${BINARY}" | awk '{print $1}')
    fi
    if [ -n "$ACTUAL" ] && [ "$EXPECTED" != "$ACTUAL" ]; then
      echo "Error: checksum verification failed"
      echo "  Expected: $EXPECTED"
      echo "  Got:      $ACTUAL"
      exit 1
    fi
  fi
fi

# Install binary.
chmod +x "${TMPDIR}/${BINARY}"

INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ] 2>/dev/null; then
  INSTALL_DIR="${HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY_NAME}"

echo ""
echo "  Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"
echo ""
echo "  Set your token:"
echo "    export ${TOKEN_ENV_VAR}=\"your-token-here\""
echo ""
echo "  Get started:"
echo "    ${BINARY_NAME} --help"
