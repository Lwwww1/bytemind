[CmdletBinding()]
param(
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\AICoding\bin"
)

$ErrorActionPreference = 'Stop'

$targetExe = Join-Path $InstallDir 'aicoding.exe'
if (Test-Path $targetExe) {
    Remove-Item $targetExe -Force
    Write-Host "Removed $targetExe" -ForegroundColor Green
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not [string]::IsNullOrWhiteSpace($userPath)) {
    $entries = $userPath.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries) |
        Where-Object { $_.TrimEnd('\\') -ine $InstallDir.TrimEnd('\\') }
    [Environment]::SetEnvironmentVariable('Path', ($entries -join ';'), 'User')
    Write-Host "Removed $InstallDir from your user PATH." -ForegroundColor Green
}

if ((Test-Path $InstallDir) -and -not (Get-ChildItem $InstallDir -Force | Select-Object -First 1)) {
    Remove-Item $InstallDir -Force
}
