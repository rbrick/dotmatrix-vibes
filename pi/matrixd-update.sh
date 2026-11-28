#!/bin/bash
# Pulls the latest main branch from GitHub and, when it has changed, rebuilds
# matrixd and matrix-pi on this Pi, installs them and restarts matrixd. If the
# new build doesn't come up, the previous binaries are put back.
#
# Run by matrixd-update.timer. Run once by hand with:
#   sudo systemctl start matrixd-update && journalctl -u matrixd-update -f
set -euo pipefail

REPO_URL=${REPO_URL:-https://github.com/rbrick/dotmatrix-vibes.git}
BRANCH=${BRANCH:-main}
STATE_DIR=${STATE_DIR:-/var/lib/matrixd-update}
RGB_MATRIX_DIR=${RGB_MATRIX_DIR:-/opt/rpi-rgb-led-matrix}
BIN_DIR=/usr/local/bin
SRC="$STATE_DIR/src"
INSTALLED="$STATE_DIR/installed-commit"

export PATH="/usr/local/go/bin:$PATH"
export HOME="$STATE_DIR" GOPATH="$STATE_DIR/gopath" GOCACHE="$STATE_DIR/gocache"
export GOTOOLCHAIN=auto    # fetches a newer Go if go.mod ever asks for one
export GOMAXPROCS=2        # keep memory down and leave cores for the display

log() { echo "matrixd-update: $*"; }

mkdir -p "$STATE_DIR"

# Videos: YouTube changes often break old yt-dlp versions, so keep a
# standalone yt-dlp current (once a day). Failure here never stops an update.
if command -v yt-dlp >/dev/null && [ -z "$(find "$STATE_DIR/yt-dlp-checked" -mmin -1440 2>/dev/null)" ]; then
  touch "$STATE_DIR/yt-dlp-checked"
  yt-dlp --update 2>&1 | tail -1 | sed 's/^/matrixd-update: yt-dlp: /' || true
fi
if [ ! -d "$SRC/.git" ]; then
  rm -rf "$SRC"
  git clone --quiet --branch "$BRANCH" "$REPO_URL" "$SRC"
fi

git -C "$SRC" fetch --quiet origin "$BRANCH"
latest=$(git -C "$SRC" rev-parse FETCH_HEAD)
current=$(cat "$INSTALLED" 2>/dev/null || true)
if [ "$latest" = "$current" ]; then
  log "up to date at ${latest:0:7}"
  exit 0
fi
log "updating ${current:0:7} -> ${latest:0:7}"
git -C "$SRC" checkout --quiet --force "$latest"
git -C "$SRC" clean -fdxq

# Build everything into a staging directory before touching what's installed.
stage=$(mktemp -d "$STATE_DIR/stage.XXXXXX")
trap 'rm -rf "$stage"' EXIT
(cd "$SRC/go" && nice go build -trimpath -o "$stage/matrixd" ./cmd/matrixd)
make -C "$RGB_MATRIX_DIR/lib" >/dev/null
make -C "$SRC/pi" RGB_MATRIX_DIR="$RGB_MATRIX_DIR" clean >/dev/null
nice make -C "$SRC/pi" RGB_MATRIX_DIR="$RGB_MATRIX_DIR" >/dev/null
cp "$SRC/pi/matrix-pi" "$stage/matrix-pi"

# Keep the running binaries so a bad update can be undone.
for f in matrixd matrix-pi; do
  [ -f "$BIN_DIR/$f" ] && cp -p "$BIN_DIR/$f" "$STATE_DIR/$f.previous"
done
install -m 0755 "$stage/matrixd" "$BIN_DIR/matrixd"
install -m 0755 "$stage/matrix-pi" "$BIN_DIR/matrix-pi"

healthy() {
  systemctl restart matrixd
  for _ in $(seq 1 20); do
    sleep 1
    systemctl is-active --quiet matrixd || continue
    curl -fsS --max-time 2 http://127.0.0.1:8080/api/state >/dev/null && return 0
  done
  return 1
}

if ! healthy; then
  log "new build failed to start; rolling back"
  for f in matrixd matrix-pi; do
    [ -f "$STATE_DIR/$f.previous" ] && install -m 0755 "$STATE_DIR/$f.previous" "$BIN_DIR/$f"
  done
  systemctl restart matrixd
  # Record the bad commit so it isn't rebuilt every 15 minutes; the next push retries.
  echo "$latest" > "$INSTALLED"
  exit 1
fi

# Pick up changes to the service, the timer and this script from the repo.
# They take effect from the next run.
changed=0
for f in matrixd.service matrixd-update.service matrixd-update.timer; do
  if [ -f "$SRC/pi/$f" ] && ! cmp -s "$SRC/pi/$f" "/etc/systemd/system/$f"; then
    install -m 0644 "$SRC/pi/$f" "/etc/systemd/system/$f"
    changed=1
  fi
done
[ -f "$SRC/pi/matrixd-update.sh" ] && install -m 0755 "$SRC/pi/matrixd-update.sh" "$BIN_DIR/matrixd-update"
if [ "$changed" = 1 ]; then
  systemctl daemon-reload
  systemctl restart matrixd
fi

echo "$latest" > "$INSTALLED"
log "now running ${latest:0:7}: $(git -C "$SRC" log -1 --format=%s)"
