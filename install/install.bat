@echo off
setlocal EnableExtensions

rem Canonical Windows entrypoint. Resolve the repository-relative installer.
call "%~dp0..\install.bat" %*
exit /b %errorlevel%
