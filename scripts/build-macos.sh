#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p dist/ProxPaste.app/Contents/MacOS
mkdir -p dist/ProxPaste.app/Contents/Resources
CGO_ENABLED=1 go build -trimpath -ldflags='-s -w' -o dist/ProxPaste.app/Contents/MacOS/ProxPaste .
cp packaging/Info.plist dist/ProxPaste.app/Contents/Info.plist
# Let macOS encode the small ICNS representations in its native format.
# Hand-packed PNG chunks can decode as corrupt pixels at Finder list sizes.
icon_tmp=$(mktemp -d)
trap 'rm -rf "$icon_tmp"' EXIT HUP INT TERM
iconset="$icon_tmp/ProxPaste.iconset"
mkdir -p "$iconset"
for size in 16 32 128 256 512; do
  for scale in 1 2; do
    pixels=$((size * scale))
    suffix=""
    if [ "$scale" = 2 ]; then suffix="@2x"; fi
    sips -z "$pixels" "$pixels" assets/icon-1024.png --out "$iconset/icon_${size}x${size}${suffix}.png" >/dev/null
  done
done
iconutil -c icns "$iconset" -o dist/ProxPaste.app/Contents/Resources/ProxPaste.icns
codesign --force --deep --sign - dist/ProxPaste.app
ditto -c -k --sequesterRsrc --keepParent dist/ProxPaste.app "dist/ProxPaste-macOS-$(go env GOARCH).zip"
