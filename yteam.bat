@echo off
setlocal
set "YTEAM_ROOT=%~dp0"
set "YTEAM_PYTHON=%YTEAM_ROOT%runtime\.venv\Scripts\python.exe"
if not exist "%YTEAM_PYTHON%" (
  echo YTEAM is not installed in this checkout. Run: python scripts\install_yteam.py
  exit /b 2
)
"%YTEAM_PYTHON%" "%YTEAM_ROOT%scripts\yteam_tui.py" %*
exit /b %ERRORLEVEL%
