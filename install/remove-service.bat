@echo off
setlocal EnableExtensions

rem Remove/uninstall the TimeKeeper Windows NSSM service.
set "SCRIPT=%~dp0..\scripts\service\install-windows.ps1"
set "NSSM=%~dp0..\scripts\service\nssm\win64\nssm.exe"

if not exist "%SCRIPT%" (
  echo ERROR: installer implementation not found: %SCRIPT%
  exit /b 2
)
if not exist "%NSSM%" (
  echo ERROR: bundled NSSM not found: %NSSM%
  exit /b 2
)

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT%" -Action uninstall -Elevate -NssmPath "%NSSM%" %*
exit /b %errorlevel%
