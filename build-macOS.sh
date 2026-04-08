#!/bin/bash
# Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

set -e

# Configuration
PRODUCT_NAME="DKST Translator AI"
BUILD_DIR="build/bin"
APP_BUNDLE="$BUILD_DIR/$PRODUCT_NAME.app"
ENTITLEMENTS="build/darwin/entitlements.plist"

echo "=== Starting macOS Build Process ==="

# 1. Environment Cleanup and Tool Verification
echo "[1/5] Cleaning up environment and checking tools..."
rm -rf "$APP_BUNDLE"
rm -f "$BUILD_DIR/$PRODUCT_NAME"
mkdir -p "$BUILD_DIR"

if ! command -v wails &> /dev/null; then
    echo "Wails CLI not found. Installing..."
    go install github.com/wailsapp/wails/v2/cmd/wails@latest
    export PATH=$PATH:$(go env GOPATH)/bin
fi

# 2. Frontend Build (Explicit build for absolute consistency)
echo "[2/5] Building frontend assets (explicitly)..."
pushd frontend
npm install
npm run build
popd

# 3. Build with Wails (using universal binary for modern Macs)
echo "[3/5] Building application with Wails..."
wails build -platform darwin/universal -ldflags "-s -w" -v 2 -o "$PRODUCT_NAME"

# 4. Verify Bundle and Binary Names
echo "[4/5] Verifying application name..."
if [ -d "$APP_BUNDLE" ]; then
    plutil -replace CFBundleExecutable -string "$PRODUCT_NAME" "$APP_BUNDLE/Contents/Info.plist"
    plutil -replace CFBundleName -string "$PRODUCT_NAME" "$APP_BUNDLE/Contents/Info.plist"
else
    echo "Error: Application bundle not found at $APP_BUNDLE"
    exit 1
fi

# 5. Code Signing Integrity (Re-signing)
echo "[5/5] Detecting signing identity and performing code signing..."
if [ -n "$MACOS_SIGN_IDENTITY" ]; then
    SIGN_ID="$MACOS_SIGN_IDENTITY"
    echo "Using SIGN_ID from environment: $SIGN_ID"
else
    SIGN_ID=$(security find-identity -p codesigning -v | grep "Developer ID Application" | awk -F '"' '{print $2}' | head -n 1)
    if [ -z "$SIGN_ID" ]; then
        SIGN_ID=$(security find-identity -p codesigning -v | grep "Apple Development" | awk -F '"' '{print $2}' | head -n 1)
    fi
    
    if [ -z "$SIGN_ID" ]; then
        SIGN_ID="-"
        echo "Warning: No valid signing certificate found. Falling back to ad-hoc signing (-)"
    else
        echo "Found signing identity: $SIGN_ID"
    fi
fi

# Deep code signing
xattr -cr "$APP_BUNDLE"
codesign --force --options runtime --sign "$SIGN_ID" --entitlements "$ENTITLEMENTS" "$APP_BUNDLE/Contents/MacOS/$PRODUCT_NAME"
codesign --force --options runtime --deep --sign "$SIGN_ID" --entitlements "$ENTITLEMENTS" "$APP_BUNDLE"

echo "=== Build Complete: $APP_BUNDLE ==="
echo "Verification:"
codesign -vvv --deep --strict "$APP_BUNDLE"
