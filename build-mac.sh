#!/bin/bash
# Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

set -e

# Configuration
PRODUCT_NAME="DKST Translator AI"
INTERNAL_NAME="DKST-Translator-AI"
BUILD_DIR="build/bin"
APP_BUNDLE="$BUILD_DIR/$PRODUCT_NAME.app"
WAILS_BUNDLE="$BUILD_DIR/$INTERNAL_NAME.app"
ENTITLEMENTS="build/darwin/entitlements.plist"

echo "=== Starting macOS Build Process ==="

# 1. Environment Cleanup
echo "[1/5] Cleaning up environment..."
rm -rf "$BUILD_DIR/$PRODUCT_NAME.app"
rm -rf "$WAILS_BUNDLE"
rm -f "$BUILD_DIR/$INTERNAL_NAME"

# 2. Build with Wails (using space-free name for stability)
echo "[2/5] Building application with Wails..."
wails build -platform darwin/universal -v 2

# 3. Rename Bundle and Binary for Production (Addressing spaces requirement)
echo "[3/5] Customizing application name with spaces..."
mv "$WAILS_BUNDLE" "$APP_BUNDLE"
mv "$APP_BUNDLE/Contents/MacOS/$INTERNAL_NAME" "$APP_BUNDLE/Contents/MacOS/$PRODUCT_NAME"

# Update Info.plist
# CFBundleExecutable and CFBundleName must match the new name
plutil -replace CFBundleExecutable -string "$PRODUCT_NAME" "$APP_BUNDLE/Contents/Info.plist"
plutil -replace CFBundleName -string "$PRODUCT_NAME" "$APP_BUNDLE/Contents/Info.plist"

# 4. Signing Identity Detection
echo "[4/5] Detecting signing identity..."
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

# 5. Code Signing Integrity (Re-signing)
echo "[5/5] Performing deep code signing with custom name..."
xattr -cr "$APP_BUNDLE"

# Sign the main binary (specifically the one with spaces)
codesign --force --options runtime --sign "$SIGN_ID" --entitlements "$ENTITLEMENTS" "$APP_BUNDLE/Contents/MacOS/$PRODUCT_NAME"

# Final deep sign for the entire bundle
codesign --force --options runtime --deep --sign "$SIGN_ID" --entitlements "$ENTITLEMENTS" "$APP_BUNDLE"

echo "=== Build Complete: $APP_BUNDLE ==="
echo "Verification:"
codesign -vvv --deep --strict "$APP_BUNDLE"
