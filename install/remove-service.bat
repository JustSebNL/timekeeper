@echo off
setlocal EnableExtensions

rem Remove/uninstall the TimeKeeper Windows service without deleting project data.
set "SCRIPT=%~dp0..\scripts\service\install-windows.ps1"

if not exist "%SCRIPT%" (
  echo ERROR: installer implementation not found: %SCRIPT%
  exit /b 2
)

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT%" -Action uninstall -Elevate %*
exit /b %errorlevel%
