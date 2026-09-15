#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="$ROOT_DIR/releases/windows-python"
BUILD_DIR="$ROOT_DIR/build"
ARCHIVE="$OUT_DIR/content-list-generator-windows-python.zip"
TMP_ARCHIVE="$BUILD_DIR/content-list-generator-windows-python.zip"

mkdir -p "$OUT_DIR" "$BUILD_DIR"
find "$OUT_DIR" -mindepth 1 -maxdepth 1 \
  ! -name '.gitkeep' \
  -exec rm -rf {} +
rm -f "$ARCHIVE" "$TMP_ARCHIVE"

cd "$ROOT_DIR"

./scripts/parity_check.sh
# Some downstream deployment copies do not include tests; skip discovery there.
if [ -d "./python/tests" ] && ls ./python/tests/test_*.py >/dev/null 2>&1; then
  python3 -m unittest discover -s ./python/tests -p 'test_*.py'
else
  echo "Skipping python/tests discovery — directory absent in this checkout."
fi
python3 -m py_compile ./python/content_list_core.py ./python/content_list_generator.py ./python/deps_check.py

cp python/content_list_core.py "$OUT_DIR/"
cp python/content_list_generator.py "$OUT_DIR/"
cp python/deps_check.py "$OUT_DIR/"
cp README.md "$OUT_DIR/"
cp requirements.txt "$OUT_DIR/"

cat > "$OUT_DIR/run-content-list-generator.cmd" <<'BAT'
@echo off
setlocal
set "SCRIPT=%USERPROFILE%\scripts\content_list_generator.py"
if not exist "%SCRIPT%" set "SCRIPT=%USERPROFILE%\scripts\content-list-gen\content_list_generator.py"
if not exist "%SCRIPT%" set "SCRIPT=%~dp0content_list_generator.py"
python "%SCRIPT%"
BAT

cat > "$OUT_DIR/content-list-generator.bat" <<'BAT'
@echo off

REM ---------------------------------------------------------------------------
REM  Content List Toolkit - GUI Launcher
REM  Requires: Python 3.x, tkinter, customtkinter
REM  Install deps (run once): pip install -r requirements.txt
REM ---------------------------------------------------------------------------

set "SCRIPT=%USERPROFILE%\scripts\content_list_generator.py"
if not exist "%SCRIPT%" set "SCRIPT=%USERPROFILE%\scripts\content-list-gen\content_list_generator.py"
if not exist "%SCRIPT%" set "SCRIPT=%~dp0content_list_generator.py"

if not exist "%SCRIPT%" (
    echo.
    echo ERROR: could not find content_list_generator.py.
    echo Looked for:
    echo   %USERPROFILE%\scripts\content_list_generator.py
    echo   %USERPROFILE%\scripts\content-list-gen\content_list_generator.py
    echo   %~dp0content_list_generator.py
    echo.
    echo Required files:
    echo   content_list_generator.py
    echo   content_list_core.py
    echo   deps_check.py
    echo.
    pause
    exit /b 1
)

where py >nul 2>&1
if %ERRORLEVEL%==0 (
  py -3 "%SCRIPT%" %*
  exit /b %ERRORLEVEL%
)

where python >nul 2>&1
if %ERRORLEVEL%==0 (
  python "%SCRIPT%" %*
  exit /b %ERRORLEVEL%
)

echo.
echo ERROR: Python 3 was not found on this system.
echo Install Python 3, then run this launcher again.
echo.
pause
exit /b 1
BAT

cat > "$OUT_DIR/launch-content-list-generator-gui.bat" <<'BAT'
@echo off
call "%~dp0content-list-generator.bat" %*
exit /b %errorlevel%
BAT

cat > "$OUT_DIR/run-content-list-generator.bat" <<'BAT'
@echo off
call "%~dp0content-list-generator.bat" %*
exit /b %errorlevel%
BAT

cat > "$OUT_DIR/run-email-copy.cmd" <<'BAT'
@echo off
setlocal
set "SCRIPT=%USERPROFILE%\scripts\content_list_generator.py"
if not exist "%SCRIPT%" set "SCRIPT=%USERPROFILE%\scripts\content-list-gen\content_list_generator.py"
if not exist "%SCRIPT%" set "SCRIPT=%~dp0content_list_generator.py"
python "%SCRIPT%" --mode email-copy %*
BAT

cat > "$OUT_DIR/run-content-list-generator-cli.cmd" <<'BAT'
@echo off
setlocal
set "SCRIPT=%USERPROFILE%\scripts\content_list_generator.py"
if not exist "%SCRIPT%" set "SCRIPT=%USERPROFILE%\scripts\content-list-gen\content_list_generator.py"
if not exist "%SCRIPT%" set "SCRIPT=%~dp0content_list_generator.py"
python "%SCRIPT%" --cli %*
BAT

python3 - <<'PY'
from pathlib import Path
import zipfile

root = Path("releases/windows-python")
archive = Path("build/content-list-generator-windows-python.zip")
with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as zf:
    for path in sorted(root.rglob("*")):
        if path.is_file():
            zf.write(path, path.relative_to(root))
PY
mv "$TMP_ARCHIVE" "$ARCHIVE"

echo "Built Windows Python package:"
echo "  $ARCHIVE"
