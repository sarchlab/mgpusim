#!/usr/bin/env bash
#
# Publish one calibration run to the GitHub Pages dashboard on the `gh-pages`
# branch, accumulating history (each run is added under runs/<run_id>/ without
# disturbing earlier runs).
#
# Resulting layout on gh-pages:
#   calibration/index.html            (the dashboard, copied from web/)
#   calibration/index.json            (rebuilt from every runs/*/data.json)
#   calibration/runs/<run_id>/data.json
#
# Served at: https://<owner>.github.io/<repo>/calibration/
# (Pages must be enabled once: Settings -> Pages -> Deploy from branch -> gh-pages.)
#
# Usage:
#   publish_gh_pages.sh --data DATA_JSON --run-id RUN_ID
#
# Environment:
#   GH_TOKEN / GITHUB_TOKEN   token with contents:write (the workflow default works)
#   GITHUB_REPOSITORY         owner/repo (set by Actions; required)
#   GITHUB_SERVER_URL         defaults to https://github.com
#   PAGES_BRANCH              defaults to gh-pages
#   SITE_SUBDIR               defaults to calibration

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WEB_DIR="$SCRIPT_DIR/web"

DATA="" RUN_ID=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --data)   DATA="$2"; shift 2 ;;
    --run-id) RUN_ID="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done
[[ -f "$DATA" ]] || { echo "ERROR: --data file not found: $DATA" >&2; exit 1; }
[[ -n "$RUN_ID" ]] || { echo "ERROR: --run-id is required" >&2; exit 1; }
[[ -n "${GITHUB_REPOSITORY:-}" ]] || { echo "ERROR: GITHUB_REPOSITORY not set" >&2; exit 1; }

TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
[[ -n "$TOKEN" ]] || { echo "ERROR: GH_TOKEN/GITHUB_TOKEN not set" >&2; exit 1; }
SERVER="${GITHUB_SERVER_URL:-https://github.com}"
BRANCH="${PAGES_BRANCH:-gh-pages}"
SUBDIR="${SITE_SUBDIR:-calibration}"
REMOTE="https://x-access-token:${TOKEN}@${SERVER#https://}/${GITHUB_REPOSITORY}.git"

# Deep link to THIS run on the published dashboard (Pages project-site URL).
OWNER="${GITHUB_REPOSITORY%%/*}"
REPO_NAME="${GITHUB_REPOSITORY##*/}"
DASH_URL="https://${OWNER}.github.io/${REPO_NAME}/${SUBDIR}/run/${RUN_ID}"

announce_url() {  # print the run's dashboard link (and add it to the CI summary)
  echo "Dashboard: $DASH_URL"
  if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
    printf '\n**📊 [View this run in the calibration dashboard](%s)**\n' "$DASH_URL" \
      >> "$GITHUB_STEP_SUMMARY"
  fi
}

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Check out the existing gh-pages branch (so prior runs are preserved); create an
# empty orphan branch on first publish.
if git clone --quiet --branch "$BRANCH" --depth 1 "$REMOTE" "$WORK" 2>/dev/null; then
  echo "Updating existing $BRANCH."
else
  echo "Creating $BRANCH (first publish)."
  git clone --quiet --depth 1 "$REMOTE" "$WORK"
  git -C "$WORK" checkout --orphan "$BRANCH"
  git -C "$WORK" rm -rqf . 2>/dev/null || true
fi

SITE="$WORK/$SUBDIR"
mkdir -p "$SITE/runs/$RUN_ID"
cp "$WEB_DIR/index.html" "$SITE/index.html"
# SPA fallback at the Pages site root so clean deep links (…/run/<id>) resolve.
cp "$WEB_DIR/404.html" "$WORK/404.html"
cp "$DATA" "$SITE/runs/$RUN_ID/data.json"
python3 "$WEB_DIR/build_index.py" "$SITE/runs" -o "$SITE/index.json"

cd "$WORK"
git add -A
if git diff --cached --quiet; then
  echo "No changes to publish (run ${RUN_ID} already on ${BRANCH})."
  announce_url
  exit 0
fi
git -c user.name="github-actions[bot]" \
    -c user.email="github-actions[bot]@users.noreply.github.com" \
    commit -q -m "calibration: publish run ${RUN_ID}"
git push -q origin "HEAD:$BRANCH"
echo "Published run ${RUN_ID} to ${BRANCH}/${SUBDIR}/."
announce_url
