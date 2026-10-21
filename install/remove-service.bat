@echo off
setlocal EnableExtensions

rem Remove/uninstall the TimeKeeper Windows service without deleting project data.
set "SCRIPT=%~dp0..\scripts\service\install-windows.ps1"

if not exist "%SCRIPT%" goto missing_script

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT%" -Action uninstall -Elevate %*
set "RESULT=%ERRORLEVEL%"
if "%RESULT%"=="0" exit /b 0

echo.
echo ERROR: TimeKeeper service removal failed with exit code %RESULT%.
echo Re-run this file from an Administrator terminal to see the full diagnostic.
echo Uninstall details are also saved under .timekeeper\service\operations\uninstall-*.log.
echo The TimeKeeper database and project files have not been deleted by this wrapper.
pause
exit /b %RESULT%

:missing_script
echo ERROR: installer implementation not found: %SCRIPT%
pause
exit /b 2
