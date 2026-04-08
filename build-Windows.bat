@echo off
rem Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

set PRODUCT_NAME=DKST Translator AI
set BUILD_DIR=build\bin

echo === Starting Windows Build Process ===

rem 1. Environment Cleanup
echo [1/4] Cleaning up environment...
if exist "%BUILD_DIR%" rmdir /s /q "%BUILD_DIR%"
mkdir "%BUILD_DIR%"

rem 2. Frontend Build
echo [2/4] Building frontend assets...
pushd frontend
call npm install
call npm run build
popd
if %ERRORLEVEL% neq 0 (
    echo [ERROR] Frontend build failed!
    exit /b %ERRORLEVEL%
)

rem 3. Wails Build (Handles bindings, version info, and compilation)
echo [3/4] Building application with Wails...
wails build -platform windows/amd64 -ldflags "-s -w" -v 2 -o "%PRODUCT_NAME%.exe"
if %ERRORLEVEL% neq 0 (
    echo [ERROR] Wails build failed!
    exit /b %ERRORLEVEL%
)

rem 4. Final Binary Naming
echo [4/4] Verified binary name '%PRODUCT_NAME%.exe'

echo === Build Complete: %BUILD_DIR%\%PRODUCT_NAME%.exe ===
pause
