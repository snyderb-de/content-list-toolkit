Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
    throw "Windows portable builds must be created on Windows so PyInstaller can bundle a Windows executable."
}

$RootDir = Resolve-Path (Join-Path $PSScriptRoot "..")
$BuildDir = Join-Path $RootDir "build"
$ReleaseDir = Join-Path $RootDir "releases\windows-portable"
$VenvDir = Join-Path $BuildDir "windows-portable-venv"
$PyInstallerWorkDir = Join-Path $BuildDir "pyinstaller-windows-portable"
$PyInstallerDistDir = Join-Path $BuildDir "pyinstaller-windows-portable-dist"
$PortableRoot = Join-Path $ReleaseDir "content-list-generator-portable"
$PortableAppDir = Join-Path $PortableRoot "app"
$PortableDataDir = Join-Path $PortableRoot "data"
$ZipPath = Join-Path $ReleaseDir "content-list-generator-windows-portable.zip"

New-Item -ItemType Directory -Force -Path $BuildDir, $ReleaseDir | Out-Null

Get-ChildItem -Path $ReleaseDir -Force |
    Where-Object { $_.Name -ne ".gitkeep" } |
    Remove-Item -Recurse -Force

if (-not (Test-Path $VenvDir)) {
    if (Get-Command py -ErrorAction SilentlyContinue) {
        & py -3 -m venv $VenvDir
    } else {
        & python -m venv $VenvDir
    }
}

$Python = Join-Path $VenvDir "Scripts\python.exe"
if (-not (Test-Path $Python)) {
    throw "Could not find venv Python at $Python"
}

Push-Location $RootDir
try {
    & $Python -m pip install --upgrade pip
    & $Python -m pip install -r (Join-Path $RootDir "requirements.txt") -r (Join-Path $RootDir "requirements-build.txt")

    & $Python -m unittest discover -s ".\python\tests" -p "test_*.py"
    & $Python -m py_compile ".\python\content_list_core.py" ".\python\content_list_generator.py" ".\python\deps_check.py"

    Remove-Item -Recurse -Force $PyInstallerWorkDir, $PyInstallerDistDir -ErrorAction SilentlyContinue

    & $Python -m PyInstaller `
        --noconfirm `
        --clean `
        --windowed `
        --name "content-list-generator" `
        --distpath $PyInstallerDistDir `
        --workpath $PyInstallerWorkDir `
        --specpath $BuildDir `
        --paths (Join-Path $RootDir "python") `
        --collect-all customtkinter `
        --collect-all blake3 `
        ".\python\content_list_generator.py"

    $BuiltAppDir = Join-Path $PyInstallerDistDir "content-list-generator"
    if (-not (Test-Path $BuiltAppDir)) {
        throw "PyInstaller did not create the expected app folder: $BuiltAppDir"
    }

    New-Item -ItemType Directory -Force -Path $PortableAppDir, $PortableDataDir | Out-Null
    Copy-Item -Path (Join-Path $BuiltAppDir "*") -Destination $PortableAppDir -Recurse -Force

    @"
@echo off
setlocal
set "APP_DIR=%~dp0app"
set "DATA_DIR=%~dp0data"
if not exist "%DATA_DIR%" mkdir "%DATA_DIR%"
set "CONTENT_LIST_GENERATOR_SETTINGS=%DATA_DIR%\content-list-generator-settings.json"
start "" "%APP_DIR%\content-list-generator.exe" %*
"@ | Set-Content -Path (Join-Path $PortableRoot "Start Content List Toolkit.cmd") -Encoding ASCII

    @"
Content List Toolkit - Portable Windows Release

How to use:
1. Copy this whole folder to a USB drive or any Windows folder.
2. Double-click "Start Content List Toolkit.cmd".
3. The app stores portable settings in the local data folder.

Notes:
- No installer is required.
- Do not move files out of the app folder; PyInstaller one-folder apps need their bundled runtime files.
- If Windows SmartScreen warns on first launch, choose More info, then Run anyway if you trust this build.
"@ | Set-Content -Path (Join-Path $PortableRoot "README.txt") -Encoding ASCII

    @"
This folder is intentionally kept beside the app so portable settings stay with the USB copy.
"@ | Set-Content -Path (Join-Path $PortableDataDir "README.txt") -Encoding ASCII

    Compress-Archive -Path $PortableRoot -DestinationPath $ZipPath -Force

    Write-Host "Built portable Windows package:"
    Write-Host "  $ZipPath"
} finally {
    Pop-Location
}
