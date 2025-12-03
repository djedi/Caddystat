#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${MAXMIND_LICENSE_KEY:-}" ]]; then
  echo "Set MAXMIND_LICENSE_KEY env var (from https://www.maxmind.com/) to download GeoLite2."
  exit 1
fi

DEST=${1:-"./MaxMind"}
mkdir -p "$DEST"

echo "Downloading GeoLite2-City to $DEST ..."
curl -sS -L "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-City&license_key=${MAXMIND_LICENSE_KEY}&suffix=tar.gz" -o /tmp/GeoLite2-City.tar.gz
tar -xzf /tmp/GeoLite2-City.tar.gz -C /tmp
LATEST=$(tar -tzf /tmp/GeoLite2-City.tar.gz | head -1 | cut -f1 -d"/")
cp "/tmp/${LATEST}/GeoLite2-City.mmdb" "$DEST/GeoLite2-City.mmdb"
echo "Done."
