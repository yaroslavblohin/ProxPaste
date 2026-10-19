#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p dist/ProxPaste.app/Contents/MacOS
mkdir -p dist/ProxPaste.app/Contents/Resources
CGO_ENABLED=1 go build -trimpath -ldflags='-s -w' -o dist/ProxPaste.app/Contents/MacOS/ProxPaste .
cp packaging/Info.plist dist/ProxPaste.app/Contents/Info.plist
cp assets/ProxPaste.icns dist/ProxPaste.app/Contents/Resources/ProxPaste.icns
codesign --force --deep --sign - dist/ProxPaste.app
ditto -c -k --sequesterRsrc --keepParent dist/ProxPaste.app "dist/ProxPaste-macOS-$(go env GOARCH).zip"
