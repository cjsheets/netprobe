#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "$0")/.." && pwd)"
output_dir="${NETPROBE_OUTPUT_DIR:-$project_dir/dist}"
app_path="$output_dir/Netprobe.app"
version="${NETPROBE_VERSION:-0.1.0}"
dmg_path="${NETPROBE_DMG_PATH:-$output_dir/Netprobe-${version}-macOS-universal.dmg}"
identity="${NETPROBE_CODESIGN_IDENTITY:--}"
staging_dir="$(mktemp -d)"
trap 'rm -rf "$staging_dir"' EXIT

if [[ ! -d "$app_path" ]]; then
  echo "Build Netprobe.app before creating the DMG: $app_path" >&2
  exit 1
fi

mkdir -p "$(dirname "$dmg_path")"
ditto "$app_path" "$staging_dir/Netprobe.app"
ln -s /Applications "$staging_dir/Applications"

if [[ -e "$dmg_path" ]]; then
  rm -f "$dmg_path"
fi

hdiutil create \
  -volname "Netprobe" \
  -srcfolder "$staging_dir" \
  -format UDZO \
  -ov \
  "$dmg_path"
hdiutil verify "$dmg_path"

if [[ "$identity" != "-" ]]; then
  codesign --sign "$identity" --timestamp --identifier com.cjsheets.netprobe.dmg "$dmg_path"
  codesign --verify --verbose=2 "$dmg_path"
fi

echo "$dmg_path"
