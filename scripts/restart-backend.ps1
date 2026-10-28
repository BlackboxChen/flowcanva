$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskBackendDir = [IO.Path]::GetFullPath((Join-Path $taskRoot 'backend'))
$taskBinary = Join-Path $taskBackendDir '.run/flowcanva-durable.exe'
if (-not (Test-Path -LiteralPath $taskBinary)) { throw 'Build backend/.run/flowcanva-durable.exe first.' }
$taskListeners = @(Get-NetTCPConnection -State Listen -LocalPort 16789 -ErrorAction SilentlyContinue)
foreach ($taskPid in ($taskListeners.OwningProcess | Select-Object -Unique)) {
    $taskProcess = Get-Process -Id $taskPid -ErrorAction Stop
    if (-not $taskProcess.Path -or -not $taskProcess.Path.StartsWith($taskBackendDir + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Port 16789 belongs to another application (PID $taskPid); no process was stopped."
    }
    Stop-Process -Id $taskPid -ErrorAction Stop
    $taskProcess.WaitForExit(10000) | Out-Null
}
$taskLogDir = Join-Path $taskRoot '.run'
New-Item -ItemType Directory -Path $taskLogDir -Force | Out-Null
$taskTag = Get-Date -Format 'yyyyMMdd-HHmmss'
$env:DEV_MODE = 'true'
$env:ADMIN_DEV_URL = 'http://127.0.0.1:5174'
$env:CANVAS_DEV_URL = 'http://127.0.0.1:5173'
$env:PORT = ':16789'
Start-Process -FilePath $taskBinary -WorkingDirectory $taskBackendDir -WindowStyle Hidden -RedirectStandardOutput (Join-Path $taskLogDir "durable-$taskTag.out.log") -RedirectStandardError (Join-Path $taskLogDir "durable-$taskTag.err.log")
$taskDeadline = (Get-Date).AddSeconds(30)
while ((Get-Date) -lt $taskDeadline) {
    try {
        $taskCapabilities = Invoke-RestMethod 'http://127.0.0.1:16789/api/comfyui/capabilities' -TimeoutSec 3
        if ($taskCapabilities.persistent_jobs -and $taskCapabilities.durable_media) {
            Write-Output "$(Get-Date -Format o) INFO restart-backend persistent_jobs=true durable_media=true"
            & (Join-Path $PSScriptRoot 'start-local.ps1')
            exit 0
        }
    } catch { Start-Sleep -Milliseconds 300 }
}
throw "Updated backend did not become ready. Inspect logs in $taskLogDir"
