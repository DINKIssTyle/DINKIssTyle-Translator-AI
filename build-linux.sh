#!/bin/bash
# Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

set -e

# Configuration
PRODUCT_NAME="DKST Translator AI"
APP_NAME="dkst-translator-ai"
BUILD_DIR="bin"

echo "=== Starting Linux Build Process ==="

# 1. Environment Cleanup and Tool Verification
echo "[1/4] Cleaning up environment and checking tools..."
rm -f "$BUILD_DIR/$PRODUCT_NAME"
rm -f "$BUILD_DIR/$APP_NAME"
mkdir -p "$BUILD_DIR"

if ! command -v wails3 &> /dev/null || [ "$(wails3 version 2>/dev/null)" != "v3.0.0-beta.1" ]; then
    echo "Installing Wails v3.0.0-beta.1 CLI..."
    go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.1
    export PATH=$PATH:$(go env GOPATH)/bin
fi

# 2. Dependency Installation (System-level)
echo "[2/4] Checking and installing system dependencies..."
if [ -x "$(command -v apt-get)" ]; then
    if apt-cache show libwebkitgtk-6.0-dev &> /dev/null; then
        sudo apt-get update && sudo apt-get install -y libgtk-4-dev libwebkitgtk-6.0-dev build-essential pkg-config
    else
        sudo apt-get update && sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev build-essential pkg-config
    fi
elif [ -x "$(command -v dnf)" ]; then
    sudo dnf install -y gtk4-devel webkitgtk6.0-devel gcc pkg-config
elif [ -x "$(command -v pacman)" ]; then
    sudo pacman -Sy --noconfirm gtk4 webkitgtk-6.0 base-devel pkgconf
elif [ -x "$(command -v apk)" ]; then
    sudo apk add gtk4.0-dev webkitgtk-6.0-dev build-base pkgconfig
else
    echo "Warning: package manager not detected."
fi

# Wails v3 defaults to GTK4/WebKitGTK 6.0 and supports GTK3/WebKitGTK 4.1 via gtk3.
EXTRA_TAGS=""
if ! pkg-config --exists webkitgtk-6.0 && pkg-config --exists webkit2gtk-4.1; then
    echo "Detected legacy WebKitGTK 4.1; enabling the gtk3 build tag..."
    EXTRA_TAGS="EXTRA_TAGS=gtk3"
fi

# 3. Build with Wails v3 and preserve the product-facing filename.
echo "[3/4] Building application with Wails v3 as '$PRODUCT_NAME'..."
wails3 task linux:build ARCH=amd64 $EXTRA_TAGS
mv "$BUILD_DIR/$APP_NAME" "$BUILD_DIR/$PRODUCT_NAME"

echo "=== Build Complete: $BUILD_DIR/$PRODUCT_NAME ==="
