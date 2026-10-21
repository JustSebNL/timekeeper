@echo off
setlocal EnableExtensions

rem Add/install TimeKeeper as a Bend2Will-managed Windows service.
set "SCRIPT=%~dp0..\scripts\service\install-windows.ps1"

if not exist "%SCRIPT%" goto missing_script

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT%" -Action install -Elevate %*
set "RESULT=%ERRORLEVEL%"
if "%RESULT%"=="0" exit /b 0

echo.
echo ERROR: TimeKeeper service installation failed with exit code %RESULT%.
echo Re-run this file from an Administrator terminal to see the full diagnostic.
echo Installation logs are saved under .timekeeper\service\migration-*\install.log.
echo Existing database and project files are not deleted by this wrapper.
pause
exit /b %RESULT%

:missing_script
echo ERROR: installer implementation not found: %SCRIPT%
pause
exit /b 2
