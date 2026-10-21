@echo off
rem Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
rem Explicit Windows service control; the tk CLI requests UAC elevation for
rem start/stop/restart and keeps status/logs read-only.
setlocal EnableExtensions DisableDelayedExpansion

set "ACTION=%~1"
if not defined ACTION goto usage
if not "%~2"=="" goto usage

if /I "%ACTION%"=="start" goto run
if /I "%ACTION%"=="stop" goto run
if /I "%ACTION%"=="restart" goto run
if /I "%ACTION%"=="status" goto run
if /I "%ACTION%"=="logs" goto run
goto usage

:run
set "TK=%~dp0..\..\.timekeeper\app\bin\tk.exe"
if not exist "%TK%" (
  echo ERROR: TimeKeeper CLI not found: "%TK%" 1>&2
  exit /b 1
)

echo TimeKeeper service action: %ACTION%
"%TK%" service "%ACTION%"
set "RESULT=%ERRORLEVEL%"
if not "%RESULT%"=="0" echo Service action failed with exit code %RESULT%. 1>&2
exit /b %RESULT%

:usage
echo Usage: kick-service.bat ^<start^|stop^|restart^|status^|logs^> 1>&2
echo Mutating actions use the supported TimeKeeper service manager and may request UAC elevation. 1>&2
exit /b 2
