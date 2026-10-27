#!/bin/sh
# LBMesh installer — served at https://get.lbmesh.io/install.sh
set -e

REPO="lb-mesh/lbmesh"
FALLBACK_TAG="v1.0.4"
INSTALL_DIR="/usr/local/bin"

# 1. Identify Host OS and CPU Architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo "Error: Unsupported CPU architecture ($ARCH)." >&2
        exit 1
        ;;
esac

case "$OS" in
    darwin|linux) ;;
    *)
        echo "Error: LBMesh is currently supported on Darwin and Linux host architectures." >&2
        exit 1
        ;;
esac

# 2. Fetch the latest release tag from the GitHub API
echo "Checking latest release version of LBMesh..."
LATEST_TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
    | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)

if [ -z "$LATEST_TAG" ]; then
    # Fallback if the GitHub API is throttled
    LATEST_TAG="$FALLBACK_TAG"
fi

# GoReleaser archive names use the version WITHOUT the leading "v":
#   lbmesh_<version>_<os>_<arch>.tar.gz
VERSION="${LATEST_TAG#v}"

echo "Installing LBMesh ${LATEST_TAG} for ${OS}/${ARCH}..."

# 3. Download and unpack (-f makes curl fail on HTTP errors instead of saving an error page)
BINARY_URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/lbmesh_${VERSION}_${OS}_${ARCH}.tar.gz"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

if ! curl -fsSL "$BINARY_URL" -o "${TMP_DIR}/lbmesh.tar.gz"; then
    echo "Error: failed to download ${BINARY_URL}" >&2
    exit 1
fi
tar -xzf "${TMP_DIR}/lbmesh.tar.gz" -C "$TMP_DIR"

# 4. Install the binary
echo "Installing binary to ${INSTALL_DIR}/lbmesh (requires sudo)..."
sudo mkdir -p "$INSTALL_DIR"
sudo install -m 0755 "${TMP_DIR}/lbmesh" "${INSTALL_DIR}/lbmesh"

# 5. Scaffold folders immediately
"${INSTALL_DIR}/lbmesh" init

echo "LBMesh successfully installed! Run 'lbmesh status' or 'lbmesh dashboard' to begin."
