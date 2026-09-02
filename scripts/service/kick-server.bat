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

set "NSSM=%TIMEKEEPER_NSSM%"
if not defined NSSM if exist "%~dp0..\..\.timekeeper\service\nssm\nssm.exe" set "NSSM=%~dp0..\..\.timekeeper\service\nssm\nssm.exe"
if not defined NSSM if exist "D:\var\nssm\win64\nssm.exe" set "NSSM=D:\var\nssm\win64\nssm.exe"
if not defined NSSM for /f "delims=" %%I in ('where nssm.exe 2^>nul') do if not defined NSSM set "NSSM=%%I"

if defined NSSM (
  "%NSSM%" restart TimeKeeper >nul 2>&1
  exit /b !ERRORLEVEL!
)

rem Fallback for a WSL-launched binary when NSSM is unavailable.
wsl.exe --exec bash -lc "cd /mnt/d/dev/codebase/dev/TimeKeeper && ./scripts/service/kick-server.sh" >nul 2>&1
exit /b %ERRORLEVEL%
