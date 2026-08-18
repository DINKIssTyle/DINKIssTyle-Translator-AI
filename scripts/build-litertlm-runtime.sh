#!/usr/bin/env bash
set -euo pipefail

VERSION="${LITERTLM_VERSION:-0.14.0}"
PLATFORM="${LITERTLM_PLATFORM:-$(uname -s | tr '[:upper:]' '[:lower:]')}"
ARCH="${LITERTLM_ARCH:-$(uname -m)}"
case "$PLATFORM" in darwin) PLATFORM=darwin ;; linux) PLATFORM=linux ;; esac
case "$ARCH" in x86_64) ARCH=amd64 ;; aarch64) ARCH=arm64 ;; esac

BUILD_ROOT="build/litertlm-runtime/$PLATFORM-$ARCH"
OUTPUT_ROOT="third_party/litertlm/$PLATFORM-$ARCH"
python3 -m venv "$BUILD_ROOT/venv"
"$BUILD_ROOT/venv/bin/python" -m pip install --upgrade pip
"$BUILD_ROOT/venv/bin/python" -m pip install "litert-lm==$VERSION" "pyinstaller==6.15.0"

# Recent tomli wheels are compiled with mypyc. Their generated helper extension
# lives at the top level of site-packages and PyInstaller cannot infer the
# dynamically generated module name from tomli's native extension.
SITE_PACKAGES="$("$BUILD_ROOT/venv/bin/python" -c 'import sysconfig; print(sysconfig.get_paths()["purelib"])')"
PYINSTALLER_ARGS=(
  --noconfirm
  --clean
  --onefile
  --name litert-lm
  --collect-all litert_lm
  --collect-all litert_lm_cli
  --collect-all litert_lm_builder
  --collect-all tomli
  --distpath "$OUTPUT_ROOT"
  --workpath "$BUILD_ROOT/work"
  --specpath "$BUILD_ROOT"
)
while IFS= read -r MYPYC_EXTENSION; do
  MYPYC_MODULE="$(basename "$MYPYC_EXTENSION")"
  MYPYC_MODULE="${MYPYC_MODULE%%.*}"
  PYINSTALLER_ARGS+=(--hidden-import "$MYPYC_MODULE")
done < <(find "$SITE_PACKAGES" -maxdepth 1 -type f -name '*__mypyc*.so' -print)

"$BUILD_ROOT/venv/bin/pyinstaller" "${PYINSTALLER_ARGS[@]}" scripts/litertlm_entry.py
chmod +x "$OUTPUT_ROOT/litert-lm"
"$OUTPUT_ROOT/litert-lm" --version
echo "Bundled LiteRT-LM $VERSION at $OUTPUT_ROOT/litert-lm"
