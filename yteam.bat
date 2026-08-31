@echo off
setlocal
set "YTEAM_ROOT=%~dp0"
set "YTEAM_PYTHON=%YTEAM_ROOT%runtime\.venv\Scripts\python.exe"
set "QUIT_MARKER=%YTEAM_ROOT%runtime\quit.marker"
if not exist "%YTEAM_PYTHON%" (
  echo YTEAM is not installed in this checkout. Run: python scripts\install_yteam.py
  exit /b 2
)

:loop
del /q "%QUIT_MARKER%" 2>nul
"%YTEAM_PYTHON%" "%YTEAM_ROOT%scripts\yteam_tui.py" %*
set RC=%ERRORLEVEL%

rem ---- quit.marker present = user asked to quit, do NOT restart ----
if exist "%QUIT_MARKER%" (
  del /q "%QUIT_MARKER%" 2>nul
  exit /b 0
)

rem ---- marker absent = CLI crashed / auto-exited -> relaunch ----
echo.
echo  [!] YTEAM keluar sendiri (rc=%RC%) - restarting otomatis...
echo  Tekan Ctrl+C berulang kali untuk berhenti.
timeout /t 2 /nobreak >nul
goto loop
