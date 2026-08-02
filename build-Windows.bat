@echo off
rem Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

set PRODUCT_NAME=DKST Translator AI
set APP_NAME=dkst-translator-ai
set BUILD_DIR=bin

echo === Starting Windows Build Process ===

rem 1. Environment Cleanup
echo [1/4] Cleaning up environment...
if exist "%BUILD_DIR%" rmdir /s /q "%BUILD_DIR%"
mkdir "%BUILD_DIR%"

set WAILS3_VERSION=
for /f "delims=" %%i in ('wails3 version 2^>nul') do set WAILS3_VERSION=%%i
if not "%WAILS3_VERSION%"=="v3.0.0-beta.1" (
    echo Installing Wails v3.0.0-beta.1 CLI...
    go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.1
    for /f "delims=" %%i in ('go env GOPATH') do set "PATH=%PATH%;%%i\bin"
)

rem 2. Wails v3 Build (handles frontend, bindings, resources, and compilation)
echo [2/4] Building application with Wails v3...
wails3 task windows:build ARCH=amd64
if %ERRORLEVEL% neq 0 (
    echo [ERROR] Wails build failed!
    exit /b %ERRORLEVEL%
)
move /Y "%BUILD_DIR%\%APP_NAME%.exe" "%BUILD_DIR%\%PRODUCT_NAME%.exe"

rem 3. Final Binary Naming
echo [3/4] Verified binary name '%PRODUCT_NAME%.exe'

echo === Build Complete: %BUILD_DIR%\%PRODUCT_NAME%.exe ===
pause
