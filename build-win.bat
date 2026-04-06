@echo off
rem Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

set PRODUCT_NAME=DKST Translator AI
set INTERNAL_NAME=DKST-Translator-AI
set BUILD_DIR=build\bin
set VERSION_JSON=build\windows\versioninfo.json
set RESOURCE_SYSO=resource_windows.syso

echo === Starting Windows Build Process ===

rem 1. Environment Cleanup
echo [1/5] Cleaning up environment...
if exist %BUILD_DIR% rmdir /s /q %BUILD_DIR%
mkdir %BUILD_DIR%

rem 2. Resource Preparation (Metadata Injection)
echo [2/5] Preparing version info and resource...
go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
%GOPATH%\bin\goversioninfo -64 -o %RESOURCE_SYSO% %VERSION_JSON%

rem 3. Wails Bindings Generation
echo [3/5] Generating Wails bindings...
wails generate bindings

rem 4. Manual Compilation (Manual Build Strategy)
echo [4/5] Compiling application (GUI mode, stripped binary) as '%PRODUCT_NAME%'...
go build -ldflags "-H windowsgui -s -w" -o "%BUILD_DIR%\%PRODUCT_NAME%.exe" main.go

rem 5. Resource and DLL Cleanup
echo [5/5] Cleaning up temporary files...
if exist %RESOURCE_SYSO% del %RESOURCE_SYSO%

echo === Build Complete: %BUILD_DIR%\%PRODUCT_NAME%.exe ===
