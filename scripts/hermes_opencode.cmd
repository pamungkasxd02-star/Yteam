@echo off
setlocal
set "ROOT=%~dp0.."
python "%ROOT%\scripts\hermes_opencode.py" %*
exit /b %ERRORLEVEL%
