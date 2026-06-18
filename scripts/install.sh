#!/usr/bin/env bash
#
# scripts/install.sh — Download and install plane-mcp from GitHub releases.
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/bizbox-asia/plane-mcp/main/scripts/install.sh | bash
#   curl -sSL https://raw.githubusercontent.com/bizbox-asia/plane-mcp/main/scripts/install.sh | bash -s -- --version v1.0.0
#   ./scripts/install.sh [--version v1.0.0] [--dir /usr/local/bin]
#
set -euo pipefail

# ---- Configuration ----
REPO="bizbox-asia/plane-mcp"
BINARY="plane-mcp"
DEFAULT_DIR="/usr/local/bin"
VERSION=""
INSTALL_DIR=""

# ---- Color helpers ----
if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
    C_RED=$'\033[0;31m'
    C_GREEN=$'\033[0;32m'
    C_YELLOW=$'\033[0;33m'
    C_BLUE=$'\033[0;34m'
    C_BOLD=$'\033[1m'
    C_RESET=$'\033[0m'
else
    C_RED="" C_GREEN="" C_YELLOW="" C_BLUE="" C_BOLD="" C_RESET=""
fi

log()  { printf "%s==>%s %s\n" "$C_BLUE" "$C_RESET" "$*"; }
ok()   { printf "%s ✓%s %s\n" "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf "%s !%s %s\n" "$C_YELLOW" "$C_RESET" "$*" >&2; }
die()  { printf "%s ✗%s %s\n" "$C_RED" "$C_RESET" "$*" >&2; exit 1; }

# ---- Usage ----
usage() {
    cat <<EOF
Usage: $(basename "$0") [flags]

Download and install plane-mcp from GitHub releases.

Flags:
  --version VERSION    Install specific version (e.g. v1.0.0, 1.0.0)
                       Default: latest release
  --dir DIRECTORY      Installation directory (default: $DEFAULT_DIR)
  --no-color           Disable colored output
  -h, --help           Show this help

Examples:
  $(basename "$0")                           # Install latest to /usr/local/bin
  $(basename "$0") --version v1.0.0          # Install specific version
  $(basename "$0") --dir ~/.local/bin        # Install to custom directory
EOF
}

# ---- Detect OS and Architecture ----
detect_platform() {
    local os arch

    case "$(uname -s)" in
        Linux*)   os="linux" ;;
        Darwin*)  os="darwin" ;;
        FreeBSD*) os="freebsd" ;;
        MINGW*|MSYS*|CYGWIN*) os="windows" ;;
        *) die "Unsupported OS: $(uname -s)" ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64)   arch="amd64" ;;
        i386|i686)      arch="386" ;;
        aarch64|arm64)  arch="arm64" ;;
        armv7l|armhf)   arch="arm" ;;
        *) die "Unsupported architecture: $(uname -m)" ;;
    esac

    echo "${os}/${arch}"
}

# ---- Get latest release version from GitHub ----
get_latest_version() {
    local url="https://api.github.com/repos/${REPO}/releases/latest"

    if command -v curl >/dev/null 2>&1; then
        curl -sSL "$url" | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/'
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "$url" | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/'
    else
        die "Neither curl nor wget found. Install one and retry."
    fi
}

# ---- Download file ----
download() {
    local url="$1"
    local output="$2"

    if command -v curl >/dev/null 2>&1; then
        curl -sSL -f -o "$output" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$output" "$url"
    else
        die "Neither curl nor wget found. Install one and retry."
    fi
}

# ---- Arg parsing ----
while [[ $# -gt 0 ]]; do
    case "$1" in
        --version)
            VERSION="$2"
            shift 2
            ;;
        --dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        --no-color)
            C_RED="" C_GREEN="" C_YELLOW="" C_BLUE="" C_BOLD="" C_RESET=""
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            die "Unknown argument: $1 (run with --help)"
            ;;
    esac
done

# ---- Set defaults ----
INSTALL_DIR="${INSTALL_DIR:-$DEFAULT_DIR}"

# ---- Main ----
log "Detecting platform..."
PLATFORM=$(detect_platform)
OS="${PLATFORM%%/*}"
ARCH="${PLATFORM#*/}"
ok "Platform: ${OS}/${ARCH}"

# Determine binary name
BINARY_NAME="${BINARY}"
[[ "$OS" == "windows" ]] && BINARY_NAME="${BINARY}.exe"

# Get version
if [[ -z "$VERSION" ]]; then
    log "Fetching latest release version..."
    VERSION=$(get_latest_version)
    [[ -z "$VERSION" ]] && die "Failed to fetch latest version. Check network or specify --version."
fi

# Normalize version
[[ "$VERSION" =~ ^v ]] || VERSION="v${VERSION}"
ok "Version: ${VERSION}"

# Construct download URL
# Asset naming: plane-mcp_<version>_<os>_<arch>[.exe]
VERSION_NUM="${VERSION#v}"
ASSET_NAME="${BINARY}_${VERSION_NUM}_${OS}_${ARCH}"
[[ "$OS" == "windows" ]] && ASSET_NAME="${ASSET_NAME}.exe"

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_NAME}"
ok "Download URL: ${DOWNLOAD_URL}"

# Create temp directory
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# Download
log "Downloading ${BINARY} ${VERSION}..."
download "$DOWNLOAD_URL" "${TMP_DIR}/${BINARY_NAME}"
ok "Downloaded to ${TMP_DIR}/${BINARY_NAME}"

# Make executable
chmod +x "${TMP_DIR}/${BINARY_NAME}"

# Verify it runs
log "Verifying binary..."
if "${TMP_DIR}/${BINARY_NAME}" -version >/dev/null 2>&1; then
    DOWNLOADED_VERSION=$("${TMP_DIR}/${BINARY_NAME}" -version 2>&1 | head -1)
    ok "Verified: ${DOWNLOADED_VERSION}"
else
    warn "Could not verify binary (may still work)"
fi

# Install
log "Installing to ${INSTALL_DIR}..."
mkdir -p "$INSTALL_DIR"

if [[ -w "$INSTALL_DIR" ]]; then
    mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
else
    warn "Directory not writable, using sudo..."
    sudo mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
fi
ok "Installed: ${INSTALL_DIR}/${BINARY_NAME}"

# Check if in PATH
if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
    warn "${INSTALL_DIR} is not in your PATH."
    warn "Add it: export PATH=\"${INSTALL_DIR}:\$PATH\""
    warn "Or add to ~/.bashrc / ~/.zshrc"
fi

# Done
cat <<EOF

${C_BOLD}Installation complete!${C_RESET}
  Binary:  ${INSTALL_DIR}/${BINARY_NAME}
  Version: ${VERSION}

Quick start:
  ${BINARY} help                    # Show all commands
  ${BINARY} projects                # List projects
  ${BINARY} health                  # Check connection

Configuration (env vars):
  export PLANE_API_KEY=your-api-key
  export PLANE_WORKSPACE_SLUG=your-workspace
  export PLANE_API_HOST_URL=https://your-plane-instance.com

EOF
