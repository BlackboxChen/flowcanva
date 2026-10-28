$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskLogDir = Join-Path $taskRoot '.run'
New-Item -ItemType Directory -Path $taskLogDir -Force | Out-Null
$taskTag = Get-Date -Format 'yyyyMMdd-HHmmss'
$taskNode = (Get-Command node.exe -ErrorAction Stop).Source
$env:DEV_MODE = 'true'
$env:ADMIN_DEV_URL = 'http://127.0.0.1:5174'
$env:CANVAS_DEV_URL = 'http://127.0.0.1:5173'
$env:PORT = ':16789'

$taskServices = @(
    @{ Name = 'backend'; Port = 16789; Folder = 'backend'; Exe = (Join-Path $taskRoot 'backend/backend.exe'); Args = @(); Url = 'http://127.0.0.1:16789/' },
    @{ Name = 'canvas'; Port = 5173; Folder = 'frontend-canvas'; Exe = $taskNode; Args = @('node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5173', '--strictPort'); Url = 'http://127.0.0.1:5173/canvas/' },
    @{ Name = 'admin'; Port = 5174; Folder = 'frontend-admin'; Exe = $taskNode; Args = @('node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5174', '--strictPort'); Url = 'http://127.0.0.1:5174/' }
)

foreach ($taskService in $taskServices) {
    $taskListening = Get-NetTCPConnection -State Listen -LocalPort $taskService.Port -ErrorAction SilentlyContinue
    if ($taskListening) {
        Write-Output "$(Get-Date -Format o) INFO start-local service=$($taskService.Name) port=$($taskService.Port) already_listening=true"
        continue
    }
    $taskStart = @{
        FilePath = $taskService.Exe
        WorkingDirectory = (Join-Path $taskRoot $taskService.Folder)
        WindowStyle = 'Hidden'
        PassThru = $true
        RedirectStandardOutput = (Join-Path $taskLogDir "$($taskService.Name)-$taskTag.out.log")
        RedirectStandardError = (Join-Path $taskLogDir "$($taskService.Name)-$taskTag.err.log")
    }
    if ($taskService.Args.Count) { $taskStart.ArgumentList = $taskService.Args }
    $taskStarted = Start-Process @taskStart
    Write-Output "$(Get-Date -Format o) INFO start-local service=$($taskService.Name) pid=$($taskStarted.Id) launched=true"
}

foreach ($taskService in $taskServices) {
    $taskReady = $false
    $taskDeadline = (Get-Date).AddSeconds(30)
    while ((Get-Date) -lt $taskDeadline) {
        try {
            $taskResponse = Invoke-WebRequest -Uri $taskService.Url -UseBasicParsing -TimeoutSec 3
            $taskListeners = Get-NetTCPConnection -State Listen -LocalPort $taskService.Port -ErrorAction Stop
            if ($taskResponse.StatusCode -eq 200 -and ($taskListeners | Where-Object { $_.LocalAddress -in @('127.0.0.1', '0.0.0.0') })) {
                $taskReady = $true
                break
            }
        } catch { Start-Sleep -Milliseconds 300 }
    }
    if (-not $taskReady) { throw "Service $($taskService.Name) is unavailable. Inspect logs in $taskLogDir" }
    Write-Output "$(Get-Date -Format o) INFO start-local service=$($taskService.Name) http_status=200 ipv4_listening=true"
}
Write-Output 'Canvas: http://127.0.0.1:16789/canvas/?build=local-media-v1#/?canvas=cv_7306ce4b'
