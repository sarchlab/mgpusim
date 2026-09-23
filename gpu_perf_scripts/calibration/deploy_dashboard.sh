#!/usr/bin/env bash
#
# Deploy the STATIC calibration dashboard to GitHub Pages (gh-pages branch).
#
# Run data now lives entirely in Firestore (the workflow streams it there and the
# dashboard reads it directly) -- the branch only hosts the static page. So run
# this ONLY when the dashboard CODE (web/index.html, web/404.html) changes, not
# per run. It also clears any stale per-run data left on the branch by the old
# publishing scheme.
#
# Served at:  https://<owner>.github.io/<repo>/calibration/
#
# Usage:  GH_TOKEN=... GITHUB_REPOSITORY=owner/repo  deploy_dashboard.sh
#
# Environment:
#   GH_TOKEN / GITHUB_TOKEN   token with contents:write
#   GITHUB_REPOSITORY         owner/repo (required)
#   GITHUB_SERVER_URL         defaults to https://github.com
#   PAGES_BRANCH              defaults to gh-pages
#   SITE_SUBDIR               defaults to calibration

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WEB_DIR="$SCRIPT_DIR/web"
TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
[[ -n "$TOKEN" ]] || { echo "ERROR: GH_TOKEN/GITHUB_TOKEN not set" >&2; exit 1; }
[[ -n "${GITHUB_REPOSITORY:-}" ]] || { echo "ERROR: GITHUB_REPOSITORY not set" >&2; exit 1; }
SERVER="${GITHUB_SERVER_URL:-https://github.com}"
BRANCH="${PAGES_BRANCH:-gh-pages}"
SUBDIR="${SITE_SUBDIR:-calibration}"
REMOTE="https://x-access-token:${TOKEN}@${SERVER#https://}/${GITHUB_REPOSITORY}.git"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

if git clone --quiet --branch "$BRANCH" --depth 1 "$REMOTE" "$WORK" 2>/dev/null; then
  echo "Updating existing $BRANCH."
else
  echo "Creating $BRANCH (first deploy)."
  git clone --quiet --depth 1 "$REMOTE" "$WORK"
  git -C "$WORK" checkout --orphan "$BRANCH"
  git -C "$WORK" rm -rqf . 2>/dev/null || true
fi

# Static site only: the dashboard page + the SPA 404 redirect. Replacing the whole
# subdir also removes stale per-run data (runs/, index.json) from the old scheme.
rm -rf "$WORK/$SUBDIR"
mkdir -p "$WORK/$SUBDIR"
cp "$WEB_DIR/index.html" "$WORK/$SUBDIR/index.html"
cp "$WEB_DIR/404.html"   "$WORK/404.html"

cd "$WORK"
git add -A
if git diff --cached --quiet; then
  echo "No dashboard changes to deploy."
  exit 0
fi
git -c user.name="github-actions[bot]" \
    -c user.email="github-actions[bot]@users.noreply.github.com" \
    commit -q -m "calibration: deploy dashboard"
git push -q origin "HEAD:$BRANCH"
echo "Deployed dashboard -> https://${GITHUB_REPOSITORY%%/*}.github.io/${GITHUB_REPOSITORY##*/}/${SUBDIR}/"
