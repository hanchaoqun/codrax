@echo off
setlocal
where pwsh >nul 2>nul
if errorlevel 1 (
  echo pwsh ^(PowerShell 7+^) is required to run eval\run.ps1. 1>&2
  exit /b 1
)
pwsh -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0run.ps1" %*
exit /b %ERRORLEVEL%
