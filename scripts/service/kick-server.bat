@echo off
rem Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
rem TIMEKEEPER_FAILURE_ONLY_SERVICE_KICKER
setlocal EnableExtensions EnableDelayedExpansion

set "ADDR=%TIMEKEEPER_ADDR%"
if not defined ADDR set "ADDR=127.0.0.1:1618"

rem Keep the healthy path silent. PowerShell is hidden so Task Scheduler does
rem not flash a console or steal focus during the normal check.
powershell.exe -NoProfile -NonInteractive -WindowStyle Hidden -Command "$ErrorActionPreference='Stop'; try { if ((Invoke-RestMethod -Uri 'http://%ADDR%/health' -TimeoutSec 4).status -eq 'ok') { exit 0 } } catch {}; exit 1"
if not errorlevel 1 exit /b 0

rem Delegate only the failed-health recovery to tk service, which uses the
rem supported service manager and requests UAC elevation when required.
set "TK=%~dp0..\..\.timekeeper\app\bin\tk.exe"
if exist "%TK%" (
  "%TK%" service restart
  exit /b !ERRORLEVEL!
)

rem Fallback for a WSL-launched binary when NSSM is unavailable.
wsl.exe --cd "%~dp0..\.." --exec bash -lc "./scripts/service/kick-server.sh" >nul 2>&1
exit /b %ERRORLEVEL%
