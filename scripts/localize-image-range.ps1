[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Canvas,
    [Parameter(Mandatory = $true)][int]$Start,
    [Parameter(Mandatory = $true)][int]$End,
    [string]$BaseUrl = 'http://localhost:16789'
)

$ErrorActionPreference = 'Stop'
$BaseUrl = $BaseUrl.TrimEnd('/')
$projectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$uploadDir = [IO.Path]::GetFullPath((Join-Path $projectRoot 'release\flowcanva\uploads\generated'))
$allowedRoot = [IO.Path]::GetFullPath((Join-Path $projectRoot 'release\flowcanva\uploads'))
if (-not $uploadDir.StartsWith($allowedRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw "本地图片目录超出项目 uploads 范围：$uploadDir"
}
New-Item -ItemType Directory -Path $uploadDir -Force | Out-Null

function Invoke-FlowCanva {
    param([string]$Method, [string]$Path, [object]$Body)
    $request = @{ Method = $Method; Uri = "$BaseUrl$Path"; ErrorAction = 'Stop' }
    if ($PSBoundParameters.ContainsKey('Body')) {
        $request.ContentType = 'application/json; charset=utf-8'
        $request.Body = $Body | ConvertTo-Json -Depth 20 -Compress
    }
    Invoke-RestMethod @request
}

$canvasData = Invoke-FlowCanva GET "/api/canvases/$Canvas"
$imageNodes = @($canvasData.nodes | Where-Object node_type -eq 'image')
if ($Start -lt 1 -or $End -lt $Start -or $End -gt $imageNodes.Count) {
    throw "图片节点范围无效：$Start-$End；画布共有 $($imageNodes.Count) 个图片节点"
}

$urlMap = @{}
$items = @()
for ($index = $Start - 1; $index -lt $End; $index++) {
    $node = $imageNodes[$index]
    $content = $node.content | ConvertFrom-Json
    foreach ($image in @($content.generated_images)) {
        $remoteUrl = [string]$image.url
        if ([string]::IsNullOrWhiteSpace($remoteUrl) -or $remoteUrl.StartsWith('/uploads/generated/')) {
            $items += [pscustomobject]@{ number = $index + 1; id = $node.id; status = 'already-local'; url = $remoteUrl }
            continue
        }
        $tempFile = [IO.Path]::GetTempFileName()
        try {
            Invoke-WebRequest -Uri $remoteUrl -OutFile $tempFile -UseBasicParsing -TimeoutSec 90
            $hash = (Get-FileHash -LiteralPath $tempFile -Algorithm SHA256).Hash.ToLowerInvariant().Substring(0, 20)
            $extension = [IO.Path]::GetExtension(([Uri]$remoteUrl).AbsolutePath).ToLowerInvariant()
            if ($extension -notin @('.png', '.jpg', '.jpeg', '.webp', '.gif', '.avif')) { $extension = '.png' }
            if ($extension -eq '.jpeg') { $extension = '.jpg' }
            $fileName = "generated_$hash$extension"
            $target = Join-Path $uploadDir $fileName
            if (Test-Path -LiteralPath $target) {
                Remove-Item -LiteralPath $tempFile -Force
            } else {
                Move-Item -LiteralPath $tempFile -Destination $target
            }
            $localUrl = "/uploads/generated/$fileName"
            $urlMap[$remoteUrl] = $localUrl
            $items += [pscustomobject]@{ number = $index + 1; id = $node.id; status = 'saved'; url = $localUrl }
        } catch {
            if (Test-Path -LiteralPath $tempFile) { Remove-Item -LiteralPath $tempFile -Force }
            $items += [pscustomobject]@{ number = $index + 1; id = $node.id; status = 'failed'; url = $remoteUrl; error = $_.Exception.Message }
        }
    }
}

$updatedNodes = 0
foreach ($node in $canvasData.nodes) {
    $updatedContent = [string]$node.content
    foreach ($remoteUrl in $urlMap.Keys) {
        $updatedContent = $updatedContent.Replace($remoteUrl, $urlMap[$remoteUrl])
    }
    if ($updatedContent -ne [string]$node.content) {
        $null = Invoke-FlowCanva PUT "/api/canvases/$Canvas/nodes/$($node.id)" @{ content = $updatedContent }
        $updatedNodes++
    }
}

[pscustomobject]@{
    canvas = $Canvas
    range = "$Start-$End"
    localized = $urlMap.Count
    updated_nodes = $updatedNodes
    failed = @($items | Where-Object status -eq 'failed').Count
    items = $items
} | ConvertTo-Json -Depth 6
