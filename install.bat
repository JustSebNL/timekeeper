@echo off
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\service\install-windows.ps1" -Build -Elevate %*
exit /b %errorlevel%
