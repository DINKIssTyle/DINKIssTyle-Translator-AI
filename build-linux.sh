#!/bin/bash
# Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

set -e

# Configuration
PRODUCT_NAME="DKST Translator AI"
INTERNAL_NAME="DKST-Translator-AI"
BUILD_DIR="build/bin"

echo "=== Starting Linux Build Process ==="

# 1. Environment Detection and Dependency Installation
echo "[1/4] Checking and installing dependencies..."
if [ -x "$(command -v apt-get)" ]; then
    sudo apt-get update && sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev build-essential pkg-config
elif [ -x "$(command -v dnf)" ]; then
    sudo dnf install -y gtk3-devel webkit2gtk3-devel build-essential pkg-config
elif [ -x "$(command -v pacman)" ]; then
    sudo pacman -Sy --noconfirm gtk3 webkit2gtk build-essential pkg-config
elif [ -x "$(command -v apk)" ]; then
    sudo apk add gtk+3.0-dev webkit2gtk-dev build-base pkgconfig
else
    echo "Warning: package manager not detected."
fi

# 2. WebKit2GTK Version Detection and Build Tags
echo "[2/4] Detecting WebKit2GTK version..."
BUILD_TAGS=""
if pkg-config --exists webkit2gtk-4.1; then
    BUILD_TAGS="-tags webkit2_41"
fi

# 3. Environment Configuration
echo "[3/4] Configuring Go environment..."
export PATH=$PATH:/usr/local/go/bin:$(go env GOPATH)/bin

if ! command -v wails &> /dev/null; then
    go install github.com/wailsapp/wails/v2/cmd/wails@latest
fi

# 4. Build with Wails and Manual Rename
echo "[4/4] Building and renaming to '$PRODUCT_NAME'..."
wails build -platform linux/amd64 $BUILD_TAGS -v 2

# Rename binary to include spaces
mv "$BUILD_DIR/$INTERNAL_NAME" "$BUILD_DIR/$PRODUCT_NAME"

echo "=== Build Complete: $BUILD_DIR/$PRODUCT_NAME ==="
