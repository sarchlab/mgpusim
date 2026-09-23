#!/usr/bin/env bash
# Install firebase-admin for the calibration dashboard publisher and print, as the
# ONLY line on stdout, the python interpreter to invoke fb_publish.py with. All
# other output goes to stderr. Exits nonzero with empty stdout if no install
# method works, so callers can cleanly skip Firestore streaming.
#
# Why this exists: some self-hosted runners' `python3 -m venv` produces a venv
# WITHOUT pip (ensurepip data stripped). The old `"$VENV/bin/pip" install ...`
# then died with exit 127 and -- because the calling step is continue-on-error --
# aborted *before* mark-run, silently (shown green), so the run never registered
# on the dashboard. We try, in order: the venv's pip (isolated, preferred, with
# an ensurepip bootstrap), then a --user system install, then a --user install
# with --break-system-packages (PEP 668 runners).
set -u

FBVENV="${RUNNER_TEMP:-/tmp}/fbvenv"
python3 -m venv "$FBVENV" >&2 2>&1 || true
VENV_PY="$FBVENV/bin/python"
if [ -x "$VENV_PY" ]; then
  "$VENV_PY" -m ensurepip --upgrade >&2 2>&1 || true
fi

if [ -x "$VENV_PY" ] && "$VENV_PY" -m pip install --quiet firebase-admin >&2 2>&1; then
  echo "$VENV_PY"
elif python3 -m pip install --quiet --user firebase-admin >&2 2>&1; then
  command -v python3
elif python3 -m pip install --quiet --user --break-system-packages firebase-admin >&2 2>&1; then
  command -v python3
else
  echo "setup_fb: failed to install firebase-admin by any method" >&2
  exit 1
fi
