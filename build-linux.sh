#!/bin/bash
# Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

set -e

# Configuration
PRODUCT_NAME="DKST Translator AI"
INTERNAL_NAME="DKST-Translator-AI"
BUILD_DIR="build/bin"

echo "=== Starting Linux Build Process ==="

# 1. Environment Cleanup and Tool Verification
echo "[1/4] Cleaning up environment and checking tools..."
rm -rf "$BUILD_DIR/$PRODUCT_NAME"
rm -f "$BUILD_DIR/$INTERNAL_NAME"
mkdir -p "$BUILD_DIR"

if ! command -v wails &> /dev/null; then
    echo "Wails CLI not found. Installing..."
    go install github.com/wailsapp/wails/v2/cmd/wails@latest
    export PATH=$PATH:$(go env GOPATH)/bin
fi

# 2. Dependency Installation (System-level)
echo "[2/4] Checking and installing system dependencies..."
if [ -x "$(command -v apt-get)" ]; then
    # Attempt to install libwebkit2gtk-4.1-dev first (for newer Ubuntu versions like 24.04)
    if apt-cache show libwebkit2gtk-4.1-dev &> /dev/null; then
        sudo apt-get update && sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev build-essential pkg-config
    else
        sudo apt-get update && sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev build-essential pkg-config
    fi
elif [ -x "$(command -v dnf)" ]; then
    sudo dnf install -y gtk3-devel webkit2gtk3-devel build-essential pkg-config
elif [ -x "$(command -v pacman)" ]; then
    sudo pacman -Sy --noconfirm gtk3 webkit2gtk build-essential pkg-config
elif [ -x "$(command -v apk)" ]; then
    sudo apk add gtk+3.0-dev webkit2gtk-dev build-base pkgconfig
else
    echo "Warning: package manager not detected."
fi

# Detect WebKit2GTK version for build tags
BUILD_TAGS=""
if pkg-config --exists webkit2gtk-4.1; then
    echo "Detected WebKit2GTK 4.1, enabling specialized tags..."
    BUILD_TAGS="-tags webkit2_41"
fi

# 3. Frontend Build (Explicit build for absolute consistency)
echo "[3/4] Building frontend assets (explicitly)..."
pushd frontend
npm install
npm run build
popd

# 4. Build with Wails and Manual Rename
echo "[4/4] Building application with Wails and renaming to '$PRODUCT_NAME'..."
wails build -platform linux/amd64 $BUILD_TAGS -ldflags "-s -w" -v 2

# Rename binary to include spaces
if [ -f "$BUILD_DIR/$INTERNAL_NAME" ]; then
    mv "$BUILD_DIR/$INTERNAL_NAME" "$BUILD_DIR/$PRODUCT_NAME"
else
    echo "Warning: Binary '$INTERNAL_NAME' not found in $BUILD_DIR"
fi

echo "=== Build Complete: $BUILD_DIR/$PRODUCT_NAME ==="
