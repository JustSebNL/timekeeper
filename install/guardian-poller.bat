@echo off
setlocal EnableExtensions

rem Install/control the user-context Anti Slack Engine poller.
set "SCRIPT=%~dp0..\scripts\guardian\install-agent-poller.ps1"
if not exist "%SCRIPT%" (
  echo ERROR: Guardian poller installer not found: %SCRIPT%
  exit /b 2
)

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT%" %*
exit /b %errorlevel%
