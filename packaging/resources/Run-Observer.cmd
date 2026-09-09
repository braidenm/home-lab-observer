@echo off
setlocal
"%~dp0observer.exe" %*
exit /b %errorlevel%
