$ErrorActionPreference = "Stop"
$installDir = Join-Path $env:LOCALAPPDATA "AgentCollab"
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
$runner = Join-Path $installDir "agentcollab-runner.exe"
Copy-Item (Join-Path $PSScriptRoot "agentcollab-runner.exe") $runner -Force
Write-Host "Manual Runner: keep this terminal open. Ctrl+C stops it."
Write-Host "Existing pairing is reused. No login startup item is installed."
& $runner @args
if ($LASTEXITCODE -ne 0) {
  throw "AgentCollab Runner exited with code $LASTEXITCODE"
}
