@echo off
setlocal EnableExtensions

rem Add/install TimeKeeper as a Bend2Will-managed Windows service.
set "SCRIPT=%~dp0..\scripts\service\install-windows.ps1"

if not exist "%SCRIPT%" (
  echo ERROR: installer implementation not found: %SCRIPT%
  exit /b 2
)

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT%" -Action install -Elevate %*
exit /b %errorlevel%
