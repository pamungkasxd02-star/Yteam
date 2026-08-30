@echo off
setlocal
python "%~dp0scripts\yteam_tui.py" %*
exit /b %ERRORLEVEL%
