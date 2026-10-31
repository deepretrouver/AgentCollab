$ErrorActionPreference = "SilentlyContinue"
$installDir = Join-Path $env:LOCALAPPDATA "AgentCollab"
$runner = Join-Path $installDir "agentcollab-runner.exe"
$launcher = Join-Path $installDir "Start-AgentCollab-Background.vbs"
$config = Join-Path $env:USERPROFILE ".agentcollab\runner.yaml"
$credentials = Join-Path $env:USERPROFILE ".agentcollab\runner\device.json"

if (-not (Test-Path $runner) -or -not (Test-Path $config) -or -not (Test-Path $credentials)) {
  (New-Object -ComObject WScript.Shell).Popup("AgentCollab Client 尚未完成首次配对，请重新运行安装包。", 8, "AgentCollab Client", 48) | Out-Null
  exit 1
}

Get-CimInstance Win32_Process -Filter "Name = 'agentcollab-runner.exe'" |
  Where-Object { $_.CommandLine -match "\bserve\b" } |
  ForEach-Object { Stop-Process -Id $_.ProcessId -Force }

Start-Sleep -Milliseconds 350
Start-Process -FilePath (Join-Path $env:SystemRoot "System32\wscript.exe") -ArgumentList ('"' + $launcher + '"') -WindowStyle Hidden
