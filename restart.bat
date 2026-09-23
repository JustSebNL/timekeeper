@echo off
REM restart.bat — notify all active agents, countdown 30s, then restart TimeKeeper.
REM Usage: restart.bat [message]
REM Example: restart.bat "Scheduled maintenance"

setlocal

if "%~1"=="" (
    set "MSG=Server restart"
) else (
    set "MSG=%~1"
)

echo Broadcasting restart notice to all active agents...
echo Message: %MSG%

REM tk agents broadcast sends a nudge to all active agents, shows a 30s
REM countdown on one line, then restarts the service via the service manager.
tk.exe agents broadcast "%MSG%"

if %ERRORLEVEL% neq 0 (
    echo Broadcast or restart failed with exit code %ERRORLEVEL%.
    exit /b %ERRORLEVEL%
)

echo Done.
exit /b 0
