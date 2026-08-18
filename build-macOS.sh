#!/bin/bash
# Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

set -e

# Configuration
PRODUCT_NAME="DKST Translator AI"
APP_NAME="dkst-translator-ai"
BUILD_DIR="bin"
GENERATED_APP="$BUILD_DIR/$APP_NAME.app"
APP_BUNDLE="$BUILD_DIR/$PRODUCT_NAME.app"
ENTITLEMENTS="build/darwin/dkst.entitlements.plist"

echo "=== Starting macOS Build Process ==="

# 1. Environment Cleanup and Tool Verification
echo "[1/5] Cleaning up environment and checking tools..."
rm -rf "$APP_BUNDLE"
rm -rf "$GENERATED_APP"
rm -f "$BUILD_DIR/$APP_NAME"
mkdir -p "$BUILD_DIR"

if ! command -v wails3 &> /dev/null; then
    echo "Installing latest Wails v3 CLI..."
    go install github.com/wailsapp/wails/v3/cmd/wails3@latest
    export PATH=$PATH:$(go env GOPATH)/bin
fi

# 2. Build and package with Wails v3 (universal binary)
echo "[2/5] Building application with Wails v3..."
wails3 task darwin:package:universal
mv "$GENERATED_APP" "$APP_BUNDLE"

# 3. Verify Bundle and Product Name
echo "[3/4] Verifying application name..."
if [ -d "$APP_BUNDLE" ]; then
    plutil -replace CFBundleName -string "$PRODUCT_NAME" "$APP_BUNDLE/Contents/Info.plist"
else
    echo "Error: Application bundle not found at $APP_BUNDLE"
    exit 1
fi

# 4. Code Signing Integrity (Re-signing)
echo "[4/4] Detecting signing identity and performing code signing..."
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
codesign --force --options runtime --sign "$SIGN_ID" --entitlements "$ENTITLEMENTS" "$APP_BUNDLE/Contents/MacOS/$APP_NAME"
codesign --force --options runtime --deep --sign "$SIGN_ID" --entitlements "$ENTITLEMENTS" "$APP_BUNDLE"

echo "=== Build Complete: $APP_BUNDLE ==="
echo "Verification:"
codesign -vvv --deep --strict "$APP_BUNDLE"
