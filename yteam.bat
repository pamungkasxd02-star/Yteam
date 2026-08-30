@echo off
setlocal
python "%~dp0scripts\hermes_opencode.py" %*
exit /b %ERRORLEVEL%
