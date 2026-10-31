# Build the real SQLite-backed Hub, not the CGO-disabled SQLite stub.
# Run from a Windows x64 Go/Node development shell with gcc on PATH.
$ErrorActionPreference = "Stop"
$Repository = Split-Path -Parent $PSScriptRoot
$PreviousCGO = $env:CGO_ENABLED
$PreviousCC = $env:CC
Push-Location $Repository
try {
  if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    throw "Windows Server requires a MinGW-w64 gcc compiler on PATH for SQLite."
  }
  npm --prefix web run build
  if ($LASTEXITCODE -ne 0) { throw "Web build failed" }
  New-Item -ItemType Directory -Force internal/httpapi/webdist | Out-Null
  Get-ChildItem internal/httpapi/webdist | Remove-Item -Recurse -Force
  Copy-Item web/dist/* internal/httpapi/webdist/ -Recurse -Force
  New-Item -ItemType Directory -Force server-desktop/runtime | Out-Null
  $env:CGO_ENABLED = "1"
  $env:CC = "gcc"
  $BuildFlags = node scripts/native-build-info.mjs
  if ($LASTEXITCODE -ne 0) { throw "Build identity generation failed" }
  go build -ldflags "$BuildFlags" -o server-desktop/runtime/agentcollab-hub.exe ./cmd/hub
  if ($LASTEXITCODE -ne 0) { throw "Server build failed" }
  # Includes account creation and real database writes, catching SQLite stubs.
  node scripts/smoke-community.mjs server-desktop/runtime/agentcollab-hub.exe
  if ($LASTEXITCODE -ne 0) { throw "Windows Server acceptance failed" }
} finally {
  $env:CGO_ENABLED = $PreviousCGO
  $env:CC = $PreviousCC
  Pop-Location
}
