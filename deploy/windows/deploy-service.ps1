param(
    [Parameter(Mandatory=$true)][string]$ServiceName,
    [Parameter(Mandatory=$true)][string]$DeployDir,
    [Parameter(Mandatory=$true)][string]$Port
)

$ErrorActionPreference = "Stop"

$targetExe = Join-Path $DeployDir "fiszki-server.exe"
$newExe    = Join-Path $DeployDir "fiszki-server-new.exe"

Write-Host "Stopping service $ServiceName if running..."
if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}

Write-Host "Replacing binary at $targetExe..."
Move-Item -Path $newExe -Destination $targetExe -Force

if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    Write-Host "Starting Windows Service $ServiceName..."
    Start-Service -Name $ServiceName
} else {
    Write-Warning "Windows Service '$ServiceName' is not registered yet. Run .\deploy\windows\setup-laptop.ps1 as Administrator once on the laptop."
    exit 1
}

Write-Host "Waiting for health check on http://localhost:$Port/healthz ..."
$healthy = $false
for ($i = 1; $i -le 15; $i++) {
    Start-Sleep -Seconds 2
    try {
        $resp = Invoke-RestMethod -Uri "http://localhost:$Port/healthz" -TimeoutSec 3
        if ($resp.status -eq "ok") {
            $healthy = $true
            break
        }
    } catch {
        Write-Host "Attempt $i/15: waiting for server..."
    }
}

if (-not $healthy) {
    Write-Error "Health check failed on port $Port!"
    exit 1
}

Write-Host "$ServiceName is up and healthy on port $Port!"
