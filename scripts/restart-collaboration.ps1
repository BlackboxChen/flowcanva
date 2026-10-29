$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$backendRoot = Join-Path $projectRoot 'backend'
$newBinary = Join-Path $backendRoot '.run\flowcanva-collaboration.exe'
if (!(Test-Path -LiteralPath $newBinary)) { throw 'New backend executable does not exist' }
$listeners = @(Get-NetTCPConnection -LocalPort 16789 -State Listen -ErrorAction SilentlyContinue)
$allowed = @((Join-Path $backendRoot '.run\flowcanva-durable.exe'), $newBinary)
foreach ($listener in $listeners) {
    $process = Get-Process -Id $listener.OwningProcess
    if ($process.Path -notin $allowed) { throw "Port 16789 belongs to another executable: $($process.Path)" }
}
foreach ($listener in $listeners) { Stop-Process -Id $listener.OwningProcess }
$env:DEV_MODE = 'true'
$env:EMBEDDED = 'false'
$env:ADMIN_DEV_URL = 'http://127.0.0.1:5174'
$env:CANVAS_DEV_URL = 'http://127.0.0.1:5173'
$env:PORT = ':16789'
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$started = Start-Process -FilePath $newBinary -WorkingDirectory $backendRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $projectRoot ".run\collaboration-$stamp.out.log") -RedirectStandardError (Join-Path $projectRoot ".run\collaboration-$stamp.err.log")
for ($attempt = 0; $attempt -lt 15; $attempt++) {
    Start-Sleep -Seconds 1
    try {
        $settings = Invoke-RestMethod 'http://127.0.0.1:16789/api/collaboration/settings'
        @{event='collaboration_backend_ready'; process_id=$started.Id; default_max_active=$settings.default_max_active; url='http://127.0.0.1:16789/canvas/'} | ConvertTo-Json
        exit 0
    } catch { if ($started.HasExited) { throw "Backend exited; inspect .run/collaboration-$stamp.err.log" } }
}
throw 'Backend did not become healthy in 15 seconds; inspect the startup logs'
