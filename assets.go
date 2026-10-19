package main

import _ "embed"

//go:generate go run ./cmd/assets

//go:embed assets/icon-256.png
var iconPNG []byte

//go:embed assets/tray.png
var trayPNG []byte

//go:embed assets/icon-1024.png
var appPNG []byte
