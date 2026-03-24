[CmdletBinding()]
param(
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\AICoding\bin",
    [string]$GoCacheDir = "",
    [switch]$SkipPathUpdate
)

$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if ([string]::IsNullOrWhiteSpace($GoCacheDir)) {
    $GoCacheDir = Join-Path $env:TEMP 'aicoding-gocache'
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'Go is not installed or not available in PATH.'
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path $GoCacheDir | Out-Null

$targetExe = Join-Path $InstallDir 'aicoding.exe'
Write-Host "Building AICoding from $repoRoot" -ForegroundColor Cyan
$env:GOCACHE = $GoCacheDir
& go -C $repoRoot build -o $targetExe .\cmd\aicoding
if ($LASTEXITCODE -ne 0) {
    throw 'go build failed.'
}

Write-Host "Installed to $targetExe" -ForegroundColor Green

if (-not $SkipPathUpdate) {
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $pathEntries = @()
    if (-not [string]::IsNullOrWhiteSpace($userPath)) {
        $pathEntries = $userPath.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries)
    }

    $alreadyPresent = $false
    foreach ($entry in $pathEntries) {
        if ($entry.TrimEnd('\') -ieq $InstallDir.TrimEnd('\')) {
            $alreadyPresent = $true
            break
        }
    }

    if (-not $alreadyPresent) {
        $newUserPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $InstallDir } else { "$userPath;$InstallDir" }
        [Environment]::SetEnvironmentVariable('Path', $newUserPath, 'User')
        if (-not (($env:Path.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries)) -contains $InstallDir)) {
            $env:Path = "$env:Path;$InstallDir"
        }
        Write-Host "Added $InstallDir to your user PATH." -ForegroundColor Green
        Write-Host 'Open a new terminal after install if the current one does not see the new command.' -ForegroundColor Yellow
    } else {
        Write-Host "$InstallDir is already in your user PATH." -ForegroundColor DarkGray
    }
}

Write-Host ''
Write-Host 'Usage examples:' -ForegroundColor Cyan
Write-Host '  aicoding chat' -ForegroundColor White
Write-Host '  aicoding run -prompt "analyze this repo"' -ForegroundColor White
Write-Host '  aicoding chat -workspace E:\experiments' -ForegroundColor White
