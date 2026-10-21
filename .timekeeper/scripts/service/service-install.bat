@echo off
rem TimeKeeper Windows service wrapper (NSSM)
rem Installs TimeKeeper as a real Windows service, then cleans up the legacy 30-minute runner.
rem Requests UAC before service mutations when not already elevated.
setlocal EnableDelayedExpansion

powershell.exe -NoProfile -NonInteractive -Command "$identity=[Security.Principal.WindowsIdentity]::GetCurrent(); $principal=New-Object Security.Principal.WindowsPrincipal($identity); if($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){exit 0}else{exit 1}" >nul 2>&1
if not errorlevel 1 goto elevated

set "ELEVATE=%~dp0..\..\..\scripts\service\elevate-batch.ps1"
if not exist "%ELEVATE%" (
  echo [service] Elevation helper not found: "%ELEVATE%" 1>&2
  exit /b 2
)
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%ELEVATE%" -BatchPath "%~f0" -WorkingDirectory "%CD%"
set "RESULT=%ERRORLEVEL%"
exit /b %RESULT%

:elevated

set "ROOT=%~dp0.."
set "NSSM=%ROOT%\service\nssm\nssm.exe"
if not exist "%NSSM%" set "NSSM=\mnt\d\var\nssm\win64\nssm.exe"
if not exist "%NSSM%" (
  echo [service] NSSM not found; set TIMEKEEPER_NSSM to the nssm.exe path.
  exit /b 69
)
set "BIN=%ROOT%\app\bin\timekeeper.exe"
set "CLI=%ROOT%\app\bin\tk.exe"
set "SERVICE=TimeKeeper"
echo [service] Using NSSM: %NSSM%

"%NSSM%" status "%SERVICE%" >nul 2>nul
if %errorlevel% equ 0 (
  echo [service] Existing service found - removing first
  "%NSSM%" stop "%SERVICE%" >nul 2>nul
  "%NSSM%" remove "%SERVICE%" confirm >nul 2>nul
  timeout /t 2 /nobreak >nul
)

"%NSSM%" install "%SERVICE%" "%BIN%"
"%NSSM%" set "%SERVICE%" AppDirectory "%ROOT%"
"%NSSM%" set "%SERVICE%" AppParameters "-addr 127.0.0.1:1618 -db .timekeeper/timekeeper.db -ui .timekeeper/web/index.html -pulse-guardian-interval 5m -proxy-addr 127.0.0.1:80"
"%NSSM%" set "%SERVICE%" DisplayName "TimeKeeper — Project Execution Memory"
"%NSSM%" set "%SERVICE%" Description "Local project execution memory for AI agents. Loopback 127.0.0.1:1618."
"%NSSM%" set "%SERVICE%" Start SERVICE_DELAYED_AUTO_START
"%NSSM%" set "%SERVICE%" AppStdout "%ROOT%\log\service.log"
"%NSSM%" set "%SERVICE%" AppStderr "%ROOT%\log\service.error.log"
"%NSSM%" set "%SERVICE%" AppStdoutCreationDisposition 4
"%NSSM%" set "%SERVICE%" AppStderrCreationDisposition 4
"%NSSM%" set "%SERVICE%" AppRotateFiles 1
"%NSSM%" set "%SERVICE%" AppRotateBytes 10485760
"%NSSM%" set "%SERVICE%" AppExit Default Restart
"%NSSM%" set "%SERVICE%" AppRestartDelay 5000
"%NSSM%" set "%SERVICE%" AppThrottle 30000
echo [service] Service installed with delayed auto-start
"%NSSM%" start "%SERVICE%"
echo [service] Started %SERVICE%; logs at %ROOT%\log\service.log

rem Disable the old 30-min task after service evidence exists
if exist "%ROOT%\service\30min-runner-old.txt" del "%ROOT%\service\30min-runner-old.txt" >nul
echo [service] Legacy 30-minute runner disabled.
endlocal
