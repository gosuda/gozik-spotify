#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
INSTALL_DIR="${HOME}/.local/share/gozik/spotify"
UNIT_DIR="${HOME}/.config/systemd/user"
UNIT_NAME="gozik-spotify.service"

mkdir -p "${INSTALL_DIR}" "${UNIT_DIR}"
cp -f "${ROOT_DIR}/gozik-spotify" "${INSTALL_DIR}/gozik-spotify"
cp -f "${ROOT_DIR}/yt-dlp" "${INSTALL_DIR}/yt-dlp"
chmod +x "${INSTALL_DIR}/gozik-spotify" "${INSTALL_DIR}/yt-dlp"
sed "s#%h/.local/share/gozik/spotify/gozik-spotify#${INSTALL_DIR}/gozik-spotify#g" \
  "${SCRIPT_DIR}/${UNIT_NAME}" > "${UNIT_DIR}/${UNIT_NAME}"

systemctl --user daemon-reload
systemctl --user enable --now "${UNIT_NAME}"
echo "gozik Spotify daemon registered and started."
