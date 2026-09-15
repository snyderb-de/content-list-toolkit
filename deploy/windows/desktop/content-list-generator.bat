@echo off

REM ---------------------------------------------------------------------------
REM  Content List Toolkit - GUI Launcher
REM  Requires: Python 3.x, tkinter, customtkinter
REM  Install deps (run once): pip install -r requirements.txt
REM
REM  Deploy scripts to:
REM    %USERPROFILE%\scripts\content-list-gen\
REM  Then double-click this .bat from Desktop.
REM ---------------------------------------------------------------------------

set "SCRIPT=%USERPROFILE%\scripts\content-list-gen\content_list_generator.py"
if not exist "%SCRIPT%" set "SCRIPT=%~dp0..\scripts\content-list-gen\content_list_generator.py"

if not exist "%SCRIPT%" (
    echo.
    echo ERROR: could not find:
    echo   %USERPROFILE%\scripts\content-list-gen\content_list_generator.py
    echo.
    echo Copy this folder to:
    echo   %USERPROFILE%\scripts\content-list-gen\
    echo.
    echo Required files:
    echo   content_list_generator.py
    echo   content_list_core.py
    echo.
    pause
    exit /b 1
)

python "%SCRIPT%" %*

REM If Python exits with an error (e.g. missing dependency), hold the window
REM open so the user can read the message before it disappears.
if %errorlevel% neq 0 (
    echo.
    echo ERROR: the script exited with code %errorlevel%.
    echo Check that Python is installed and dependencies are up to date:
    echo   pip install -r requirements.txt
    echo.
    pause
)
