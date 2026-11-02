# Run this script ONCE as Administrator in PowerShell on your Windows laptop.
# Prerequisite: Install NSSM (Non-Sucking Service Manager) via `winget install NSSM.NSSM` or `choco install nssm`

$ErrorActionPreference = "Stop"

$environments = @(
    @{ Name = "fiszki-server-prod";    Dir = "C:\fiszki\prod";    Desc = "Fiszki API Production (main branch, port 8080)" },
    @{ Name = "fiszki-server-staging"; Dir = "C:\fiszki\staging"; Desc = "Fiszki API Staging (dev branch, port 8081)" }
)

foreach ($env in $environments) {
    $svcName = $env.Name
    $dir     = $env.Dir
    $exePath = Join-Path $dir "fiszki-server.exe"
    $logDir  = Join-Path $dir "logs"

    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    New-Item -ItemType Directory -Force -Path $logDir | Out-Null

    if (-not (Get-Service -Name $svcName -ErrorAction SilentlyContinue)) {
        Write-Host "Registering Windows Service: $svcName ..."
        nssm install $svcName $exePath
        nssm set $svcName AppDirectory $dir
        nssm set $svcName Description $env.Desc
        nssm set $svcName Start SERVICE_AUTO_START
        nssm set $svcName AppStdout (Join-Path $logDir "stdout.log")
        nssm set $svcName AppStderr (Join-Path $logDir "stderr.log")
        nssm set $svcName AppRotateFiles 1
        nssm set $svcName AppRotateBytes 10485760
        Write-Host "Registered $svcName."
    } else {
        Write-Host "Service $svcName already exists."
    }
}

Write-Host "`nDone! Both services are registered to start automatically on Windows boot."
