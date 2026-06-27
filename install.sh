#!/bin/sh
set -e

REPO="deckplane/deckplane-cli"
BIN_NAME="deckplane"

# Determine OS
OS="$(uname -s)"
case "${OS}" in
    Linux*)     OS_NAME=linux;;
    Darwin*)    OS_NAME=darwin;;
    CYGWIN*|MINGW*|MSYS*) OS_NAME=windows;;
    *)          echo "Unsupported OS: ${OS}"; exit 1;;
esac

# Determine architecture
ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64*|amd64*) ARCH_NAME=amd64;;
    aarch64*|arm64*) ARCH_NAME=arm64;;
    *)               echo "Unsupported architecture: ${ARCH}"; exit 1;;
esac

TARGET="${OS_NAME}-${ARCH_NAME}"
EXT=""
if [ "${OS_NAME}" = "windows" ]; then
    EXT=".exe"
fi

echo "Detecting latest release..."
LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"
VERSION=$(curl -sL $LATEST_URL | grep '"tag_name":' | cut -d '"' -f 4)

if [ -z "$VERSION" ]; then
    echo "Failed to fetch latest version from GitHub."
    echo "You may have hit the GitHub API rate limit."
    exit 1
fi

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BIN_NAME}-${TARGET}${EXT}"

TMP_DIR="$(mktemp -d)"
TMP_BIN="${TMP_DIR}/${BIN_NAME}${EXT}"

echo "Downloading ${BIN_NAME} ${VERSION} for ${TARGET}..."
curl -sL "${DOWNLOAD_URL}" -o "${TMP_BIN}"

chmod +x "${TMP_BIN}"

INSTALL_DIR="/usr/local/bin"

if [ "${OS_NAME}" = "windows" ]; then
    INSTALL_DIR="${HOME}/bin"
    mkdir -p "${INSTALL_DIR}"
fi

echo "Installing to ${INSTALL_DIR}..."

# Check if we can write to the install directory
if [ -w "${INSTALL_DIR}" ]; then
    mv "${TMP_BIN}" "${INSTALL_DIR}/${BIN_NAME}${EXT}"
else
    echo "Administrator privileges required to write to ${INSTALL_DIR}."
    sudo mv "${TMP_BIN}" "${INSTALL_DIR}/${BIN_NAME}${EXT}"
fi

rm -rf "${TMP_DIR}"

echo ""
echo "Successfully installed ${BIN_NAME} to ${INSTALL_DIR}/${BIN_NAME}${EXT}"
if [ "${OS_NAME}" = "windows" ] || ! command -v "${BIN_NAME}" >/dev/null 2>&1; then
    echo "Make sure ${INSTALL_DIR} is in your PATH."
fi
