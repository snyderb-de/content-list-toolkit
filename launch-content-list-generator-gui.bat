@echo off

REM ---------------------------------------------------------------------------
REM  Content List Toolkit - GUI Launcher
REM  Requires: Python 3.x, tkinter, customtkinter
REM  Install deps (run once): pip install -r requirements.txt
REM
REM  Preferred deploy target:
REM    %USERPROFILE%\scripts\content-list-gen\
REM ---------------------------------------------------------------------------

set "SCRIPT=%USERPROFILE%\scripts\content-list-gen\content_list_generator.py"
if not exist "%SCRIPT%" set "SCRIPT=%USERPROFILE%\scripts\content_list_generator.py"
if not exist "%SCRIPT%" set "SCRIPT=%~dp0python\content_list_generator.py"

if not exist "%SCRIPT%" (
    echo.
    echo ERROR: could not find content_list_generator.py.
    echo Looked for:
    echo   %USERPROFILE%\scripts\content-list-gen\content_list_generator.py
    echo   %USERPROFILE%\scripts\content_list_generator.py
    echo   %~dp0python\content_list_generator.py
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
