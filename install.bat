@echo off
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\service\install-windows.ps1" -Build -Elevate -NssmPath "%~dp0scripts\service\nssm\win64\nssm.exe" %*
exit /b %errorlevel%
