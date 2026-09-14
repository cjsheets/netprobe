#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "$0")/.." && pwd)"
output_dir="${NETPROBE_OUTPUT_DIR:-$project_dir/dist}"
app_path="$output_dir/Netprobe.app"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

version="${NETPROBE_VERSION:-0.1.0}"
build_number="${NETPROBE_BUILD_NUMBER:-1}"
identity="${NETPROBE_CODESIGN_IDENTITY:--}"

if [[ -e "$app_path" ]]; then
  rm -rf "$app_path"
fi
mkdir -p "$work_dir/go" "$work_dir/swift" "$app_path/Contents/MacOS" "$app_path/Contents/Helpers" "$app_path/Contents/Resources"

for arch in arm64 amd64; do
  GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$version" -o "$work_dir/go/netprobe-$arch" "$project_dir/cmd/netprobe"
done
xcrun lipo -create "$work_dir/go/netprobe-arm64" "$work_dir/go/netprobe-amd64" -output "$app_path/Contents/Helpers/netprobe"

swift_sources=("$project_dir"/macos/NetprobeApp/*.swift)
for target in arm64 x86_64; do
  xcrun swiftc -swift-version 5 -parse-as-library -O -target "$target-apple-macos13.0" \
    -framework SwiftUI -framework Charts -framework AppKit \
    "${swift_sources[@]}" -o "$work_dir/swift/Netprobe-$target"
done
xcrun lipo -create "$work_dir/swift/Netprobe-arm64" "$work_dir/swift/Netprobe-x86_64" -output "$app_path/Contents/MacOS/Netprobe"

cp "$project_dir/macos/Info.plist" "$app_path/Contents/Info.plist"
cp "$project_dir/netprobe.example.yaml" "$app_path/Contents/Resources/netprobe.example.yaml"
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $version" "$app_path/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $build_number" "$app_path/Contents/Info.plist"

if [[ "$identity" == "-" ]]; then
  codesign --force --sign - "$app_path/Contents/Helpers/netprobe"
  codesign --force --sign - "$app_path/Contents/MacOS/Netprobe"
  codesign --force --sign - "$app_path"
else
  codesign --force --options runtime --timestamp --sign "$identity" "$app_path/Contents/Helpers/netprobe"
  codesign --force --options runtime --timestamp --sign "$identity" "$app_path/Contents/MacOS/Netprobe"
  codesign --force --options runtime --timestamp --sign "$identity" "$app_path"
fi

codesign --verify --deep --strict --verbose=2 "$app_path"
echo "$app_path"
