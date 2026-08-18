$ErrorActionPreference = "Stop"
$Version = if ($env:LITERTLM_VERSION) { $env:LITERTLM_VERSION } else { "0.14.0" }
$Arch = if ($env:LITERTLM_ARCH) { $env:LITERTLM_ARCH } else { "amd64" }
$BuildRoot = "build/litertlm-runtime/windows-$Arch"
$OutputRoot = "third_party/litertlm/windows-$Arch"

python -m venv "$BuildRoot/venv"
& "$BuildRoot/venv/Scripts/python.exe" -m pip install --upgrade pip
& "$BuildRoot/venv/Scripts/python.exe" -m pip install "litert-lm==$Version" "pyinstaller==6.15.0"
$SitePackages = & "$BuildRoot/venv/Scripts/python.exe" -c 'import sysconfig; print(sysconfig.get_paths()["purelib"])'
$PyInstallerArgs = @("--noconfirm", "--clean", "--onefile", "--name", "litert-lm", "--collect-all", "litert_lm", "--collect-all", "litert_lm_cli", "--collect-all", "litert_lm_builder", "--collect-all", "tomli", "--distpath", $OutputRoot, "--workpath", "$BuildRoot/work", "--specpath", $BuildRoot)
Get-ChildItem -Path $SitePackages -Filter "*__mypyc*.pyd" -File | ForEach-Object {
    $Module = $_.Name.Split('.')[0]
    $PyInstallerArgs += @("--hidden-import", $Module)
}
$PyInstallerArgs += "scripts/litertlm_entry.py"
& "$BuildRoot/venv/Scripts/pyinstaller.exe" @PyInstallerArgs
if ($LASTEXITCODE -ne 0) { throw "PyInstaller failed with exit code $LASTEXITCODE" }
& "$OutputRoot/litert-lm.exe" --version
if ($LASTEXITCODE -ne 0) { throw "Bundled LiteRT-LM self-check failed with exit code $LASTEXITCODE" }
Write-Host "Bundled LiteRT-LM $Version at $OutputRoot/litert-lm.exe"
