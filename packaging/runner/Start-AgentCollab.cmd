@echo off
title AgentCollab Client
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0install-windows.ps1" %*
set "EXIT_CODE=%ERRORLEVEL%"
if not "%EXIT_CODE%"=="0" (
  echo.
  echo AgentCollab setup failed. Keep this window open and send a screenshot to the administrator.
  pause
)
exit /b %EXIT_CODE%
