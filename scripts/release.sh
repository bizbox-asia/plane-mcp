#!/usr/bin/env bash
#
# scripts/release.sh — build cross-platform binaries and publish to GitHub.
#
# This wraps `make release` (which builds for 9 platforms and generates
# SHA-256 checksums) and pushes the result to a GitHub release using
# the `gh` CLI.
#
# Usage:
#   scripts/release.sh v1.2.3                    # build + release
#   scripts/release.sh v1.2.3 --dry-run          # build, don't publish
#   scripts/release.sh v1.2.3 --prerelease       # mark as pre-release
#   scripts/release.sh v1.2.3 --repo owner/name  # publish to a specific repo
#   scripts/release.sh v1.2.3 --notes-file CHANGELOG.md
#   scripts/release.sh v1.2.3 --skip-build       # upload existing artifacts
#
# Requirements:
#   - gh CLI (https://cli.github.com/) authenticated via `gh auth login`
#   - clean working tree on the branch you're releasing from
#
set -euo pipefail

# ---- locate ourselves and the project root ----
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# ---- defaults ----
VERSION=""
REPO_FLAG=""
DRY_RUN=false
SKIP_BUILD=false
PRERELEASE=false
NOTES_FILE=""
DRAFT=false

# ---- color helpers (respect NO_COLOR) ----
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

# ---- usage ----
usage() {
    cat <<EOF
Usage: $(basename "$0") <version> [flags]

Build cross-platform binaries and publish to GitHub.

Arguments:
  <version>              Semver version, with or without 'v' prefix
                         (e.g. v1.2.3 or 1.2.3)

Flags:
  --repo OWNER/NAME      Publish to a specific GitHub repo
                         (default: repo of the current directory)
  --prerelease           Mark the release as a pre-release
  --draft                Create as a draft (not published)
  --dry-run              Build but don't upload or create the release
  --skip-build           Use existing dist/release-<version>/ artifacts
  --notes-file PATH      Release notes from a file (default: auto-generate)
  --no-color             Disable colored output
  -h, --help             Show this help

Examples:
  $(basename "$0") v1.0.0
  $(basename "$0") v1.0.0-rc1 --prerelease
  $(basename "$0") v1.0.0 --repo your-org/plane-mcp --dry-run
EOF
}

# ---- arg parsing ----
# Handle --help / -h as a special case before positional parsing, so
# users can run `scripts/release.sh --help` without supplying a version.
if [[ $# -ge 1 && ( "$1" == "-h" || "$1" == "--help" ) ]]; then
    usage
    exit 0
fi

if [[ $# -lt 1 ]]; then
    usage
    exit 1
fi

VERSION="$1"
shift

while [[ $# -gt 0 ]]; do
    case "$1" in
        --repo)        REPO_FLAG="--repo $2"; shift 2 ;;
        --prerelease)  PRERELEASE=true; shift ;;
        --draft)       DRAFT=true; shift ;;
        --dry-run)     DRY_RUN=true; shift ;;
        --skip-build)  SKIP_BUILD=true; shift ;;
        --notes-file)  NOTES_FILE="$2"; shift 2 ;;
        --no-color)    C_RED="" C_GREEN="" C_YELLOW="" C_BLUE="" C_BOLD="" C_RESET=""; shift ;;
        -h|--help)     usage; exit 0 ;;
        *)             die "Unknown argument: $1 (run with --help)" ;;
    esac
done

# Normalize version: accept "1.2.3" or "v1.2.3", always store with v prefix
if [[ "$VERSION" =~ ^v?(.+)$ ]]; then
    VERSION="v${BASH_REMATCH[1]}"
fi

if ! [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
    die "Invalid version: $VERSION (expected semver, e.g. v1.2.3 or v1.2.3-rc1)"
fi

RELEASE_DIR="dist/release-${VERSION}"

# ---- pre-flight checks ----
log "Pre-flight checks"

command -v gh >/dev/null 2>&1 || die "gh CLI not found. Install: https://cli.github.com/"
gh auth status >/dev/null 2>&1 || die "gh not authenticated. Run: gh auth login"
ok "gh CLI authenticated"

command -v make >/dev/null 2>&1 || die "make not found"
ok "make available"

# Working tree must be clean (unless --skip-build is used and we don't tag).
if ! git diff --quiet HEAD 2>/dev/null; then
    die "Working tree has uncommitted changes. Commit or stash first."
fi
ok "working tree clean"

# Check the release doesn't already exist
if gh release view "$VERSION" $REPO_FLAG >/dev/null 2>&1; then
    die "Release $VERSION already exists on GitHub. Bump the version or delete the existing release."
fi
ok "release $VERSION does not exist yet"

# Check the git tag doesn't already exist locally
if git rev-parse "$VERSION" >/dev/null 2>&1; then
    die "Git tag $VERSION already exists locally. Delete it first: git tag -d $VERSION"
fi
ok "git tag $VERSION does not exist locally"

# ---- build ----
if [[ "$SKIP_BUILD" == true ]]; then
    log "Skipping build (--skip-build)"
    [[ -d "$RELEASE_DIR" ]] || die "$RELEASE_DIR not found. Run without --skip-build first."
else
    log "Building release artifacts (this takes ~2 min for 9 platforms)"
    make release VERSION="$VERSION"
    ok "build complete: $RELEASE_DIR"
fi

# Show what we're about to upload
log "Release artifacts:"
find "$RELEASE_DIR" -type f \( -name "plane-mcp*" -o -name "checksums.txt" \) -exec ls -lh {} \; | sort -k9

# ---- dry-run exit ----
if [[ "$DRY_RUN" == true ]]; then
    warn "DRY RUN: not creating release. Artifacts at $RELEASE_DIR/"
    exit 0
fi

# ---- create release ----
log "Creating GitHub release $VERSION"

# Build gh release create args
GH_ARGS=(
    "$VERSION"
    "$RELEASE_DIR/checksums.txt"
    "$RELEASE_DIR/darwin/amd64/plane-mcp"
    "$RELEASE_DIR/darwin/arm64/plane-mcp"
    "$RELEASE_DIR/freebsd/amd64/plane-mcp"
    "$RELEASE_DIR/freebsd/arm64/plane-mcp"
    "$RELEASE_DIR/linux/386/plane-mcp"
    "$RELEASE_DIR/linux/amd64/plane-mcp"
    "$RELEASE_DIR/linux/arm64/plane-mcp"
    "$RELEASE_DIR/windows/386/plane-mcp.exe"
    "$RELEASE_DIR/windows/amd64/plane-mcp.exe"
)
[[ -n "$REPO_FLAG" ]] && GH_ARGS+=($REPO_FLAG)
[[ "$PRERELEASE" == true ]] && GH_ARGS+=(--prerelease)
[[ "$DRAFT" == true ]] && GH_ARGS+=(--draft)

if [[ -n "$NOTES_FILE" ]]; then
    [[ -f "$NOTES_FILE" ]] || die "Notes file not found: $NOTES_FILE"
    GH_ARGS+=(--notes-file "$NOTES_FILE")
else
    GH_ARGS+=(--generate-notes)
fi

# gh release create prints progress to stderr; capture it for the success log
gh release create "${GH_ARGS[@]}"
ok "release $VERSION published"

# ---- post-release summary ----
DEST="GitHub"
if [[ -n "$REPO_FLAG" ]]; then
    DEST="$REPO_FLAG (--repo $REPO_FLAG)"
fi

cat <<EOF

${C_BOLD}Release summary${C_RESET}
  Version:     $VERSION
  Destination: $DEST
  Type:        $([[ $PRERELEASE == true ]] && echo "pre-release" || ([[ $DRAFT == true ]] && echo "draft" || echo "stable"))
  Artifacts:   $(find "$RELEASE_DIR" -type f \( -name "plane-mcp*" -o -name "checksums.txt" \) | wc -l | tr -d ' ') files

View at:
  $(gh release view "$VERSION" $REPO_FLAG --json url -q .url 2>/dev/null || echo "(URL will be in the gh output above)")

EOF
