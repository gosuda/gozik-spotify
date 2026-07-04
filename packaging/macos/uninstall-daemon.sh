#!/bin/bash
set -euo pipefail

PLIST_DST="${HOME}/Library/LaunchAgents/com.gosuda.gozik.spotify.plist"
launchctl unload "${PLIST_DST}" >/dev/null 2>&1 || true
rm -f "${PLIST_DST}"
echo "gozik Spotify daemon unregistered."
