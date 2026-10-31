@echo off
setlocal
set "CONFIG=%USERPROFILE%\.agentcollab\runner.yaml"
set "CREDENTIALS=%USERPROFILE%\.agentcollab\runner\device.json"
if not exist "%CONFIG%" exit /b 0
if not exist "%CREDENTIALS%" exit /b 0
"%~dp0agentcollab-runner.exe" serve --config "%CONFIG%" >> "%LOCALAPPDATA%\AgentCollab\runner.log" 2>&1
