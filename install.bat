@echo off
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0install\install-windows.ps1" -Build -Elevate %*
exit /b %errorlevel%
