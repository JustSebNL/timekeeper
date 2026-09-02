@echo off
rem Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
rem TIMEKEEPER_AGENT_HARNESS_HEALTH_GATE
setlocal EnableExtensions

set "ADDR=%TIMEKEEPER_ADDR%"
if not defined ADDR set "ADDR=127.0.0.1:1618"

powershell.exe -NoProfile -NonInteractive -WindowStyle Hidden -Command "$ErrorActionPreference='Stop'; try { if ((Invoke-RestMethod -Uri 'http://%ADDR%/health' -TimeoutSec 4).status -eq 'ok') { exit 0 } } catch {}; exit 1"
if not errorlevel 1 (
  echo TimeKeeper healthy on %ADDR%
  exit /b 0
)

call "%~dp0service\kick-server.bat"
if errorlevel 1 (
  echo TimeKeeper recovery command failed on %ADDR% 1>&2
  exit /b 1
)

for /l %%I in (1,1,10) do (
  powershell.exe -NoProfile -NonInteractive -WindowStyle Hidden -Command "$ErrorActionPreference='Stop'; try { if ((Invoke-RestMethod -Uri 'http://%ADDR%/health' -TimeoutSec 2).status -eq 'ok') { exit 0 } } catch {}; exit 1"
  if not errorlevel 1 (
    echo TimeKeeper recovered on %ADDR%
    exit /b 0
  )
  >nul timeout /t 1 /nobreak
)

echo TimeKeeper remains unavailable on %ADDR%. Inspect .timekeeper\log\ and run tk doctor when recovered. 1>&2
exit /b 1
