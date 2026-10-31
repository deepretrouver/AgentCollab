$ErrorActionPreference = "Stop"
$Repository = Split-Path -Parent $PSScriptRoot
$Runtime = Join-Path $Repository "desktop\runtime"

New-Item -ItemType Directory -Force -Path $Runtime | Out-Null

Push-Location $Repository
try {
  $PreviousCGO = $env:CGO_ENABLED
  $env:CGO_ENABLED = "0"
  $BuildFlags = node scripts/native-build-info.mjs
  if ($LASTEXITCODE -ne 0) { throw "Build identity generation failed" }
  go build -ldflags "$BuildFlags" -o (Join-Path $Runtime "agentcollab-runner.exe") ./cmd/runner
  if ($LASTEXITCODE -ne 0) { throw "Runner build failed" }
  $env:CGO_ENABLED = $PreviousCGO

  npm --prefix desktop run dist:win -- --x64
  if ($LASTEXITCODE -ne 0) { throw "Client package build failed" }
} finally {
  $env:CGO_ENABLED = $PreviousCGO
  Pop-Location
}
