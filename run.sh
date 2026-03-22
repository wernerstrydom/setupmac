#!/bin/bash
# run.sh — download, verify, and run setupmac
#
# Usage (flags are forwarded to setupmac):
#   curl -fsSL https://raw.githubusercontent.com/wernerstrydom/setupmac/main/run.sh \
#     | sudo bash -s -- [--username USER] [--vnc-password PASS] [--skip-filevault] [--dry-run]
#
# For steps that prompt for a password (FileVault, auto-login), download and
# run directly so stdin is attached to your terminal:
#   curl -fsSL https://raw.githubusercontent.com/wernerstrydom/setupmac/main/run.sh \
#     -o /tmp/run.sh && sudo bash /tmp/run.sh [flags]

set -euo pipefail

REPO="wernerstrydom/setupmac"
BIN_NAME="setupmac"
INSTALL_DIR="/usr/local/bin"

# ── Pre-flight ────────────────────────────────────────────────────────────────
if [ "$(uname -s)" != "Darwin" ]; then
    echo "error: setupmac only runs on macOS" >&2
    exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
    echo "error: this script must be run as root — use: curl ... | sudo bash" >&2
    exit 1
fi

if ! command -v curl &>/dev/null; then
    echo "error: curl is required but not found" >&2
    exit 1
fi

# ── Architecture ──────────────────────────────────────────────────────────────
case "$(uname -m)" in
    arm64)  ARCH="arm64" ;;
    x86_64) ARCH="amd64" ;;
    *)
        echo "error: unsupported architecture: $(uname -m)" >&2
        exit 1
        ;;
esac

ASSET="${BIN_NAME}-darwin-${ARCH}"

# ── Resolve latest release version ────────────────────────────────────────────
# Follow the /releases/latest redirect and extract the tag from the final URL.
VERSION="$(curl -fsSL -o /dev/null -w '%{url_effective}' \
    "https://github.com/${REPO}/releases/latest" \
    | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+')"
if [ -z "$VERSION" ]; then
    echo "error: could not determine latest release version" >&2
    exit 1
fi

BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

# ── Download ──────────────────────────────────────────────────────────────────
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

echo "==> Downloading ${BIN_NAME} ${VERSION}..."
curl -fsSL --progress-bar "${BASE_URL}/${ASSET}"      -o "${WORK_DIR}/${ASSET}"
curl -fsSL               "${BASE_URL}/checksums.txt"  -o "${WORK_DIR}/checksums.txt"

# ── Verify checksum ───────────────────────────────────────────────────────────
echo "==> Verifying checksum..."
EXPECTED="$(grep "${ASSET}" "${WORK_DIR}/checksums.txt" | awk '{print $1}')"
if [ -z "$EXPECTED" ]; then
    echo "error: ${ASSET} not found in checksums.txt" >&2
    exit 1
fi
ACTUAL="$(shasum -a 256 "${WORK_DIR}/${ASSET}" | awk '{print $1}')"
if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "error: checksum mismatch for ${ASSET}" >&2
    echo "  expected: ${EXPECTED}" >&2
    echo "  actual:   ${ACTUAL}" >&2
    exit 1
fi

# ── Install ───────────────────────────────────────────────────────────────────
mkdir -p "${INSTALL_DIR}"
chmod +x "${WORK_DIR}/${ASSET}"
mv "${WORK_DIR}/${ASSET}" "${INSTALL_DIR}/${BIN_NAME}"
echo "==> Installed ${INSTALL_DIR}/${BIN_NAME}"

# ── Run ───────────────────────────────────────────────────────────────────────
# Redirect stdin from /dev/tty so password prompts work even when this script
# is being read from a curl pipe.
exec "${INSTALL_DIR}/${BIN_NAME}" "$@" </dev/tty
