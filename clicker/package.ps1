$ErrorActionPreference = "Stop"

$clickerDir = $PSScriptRoot
$rootDir = Split-Path $clickerDir -Parent
$packagingDir = Join-Path $rootDir "packaging"
$releaseDir = Join-Path $rootDir "release"
$packageName = "BelarusChampClicker-Windows-x64"
$stagingDir = Join-Path $releaseDir $packageName
$zipPath = Join-Path $releaseDir "$packageName.zip"
$appExeName = "Belarus Champ Clicker.exe"

Write-Host "Building application..." -ForegroundColor Cyan
& (Join-Path $clickerDir "build.ps1")
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$builtExe = Join-Path $rootDir "clicker.exe"
if (-not (Test-Path $builtExe)) {
    Write-Host "Error: clicker.exe was not produced by build.ps1" -ForegroundColor Red
    exit 1
}

Write-Host "Preparing release package..." -ForegroundColor Cyan

if (Test-Path $stagingDir) {
    Remove-Item $stagingDir -Recurse -Force
}
New-Item -ItemType Directory -Path $stagingDir -Force | Out-Null

Copy-Item $builtExe (Join-Path $stagingDir $appExeName) -Force
Copy-Item (Join-Path $packagingDir "README.txt") $stagingDir -Force
Copy-Item (Join-Path $packagingDir "README.ru.txt") $stagingDir -Force
Copy-Item (Join-Path $packagingDir "Install.cmd") $stagingDir -Force
Copy-Item (Join-Path $packagingDir "Uninstall.cmd") $stagingDir -Force

if (Test-Path $zipPath) {
    Remove-Item $zipPath -Force
}
Compress-Archive -Path $stagingDir -DestinationPath $zipPath -Force

Write-Host ""
Write-Host "Release package ready:" -ForegroundColor Green
Write-Host "  Folder: $stagingDir" -ForegroundColor Yellow
Write-Host "  ZIP:    $zipPath" -ForegroundColor Yellow
Write-Host ""
Write-Host "User steps: extract ZIP -> run Install.cmd -> run Belarus Champ Clicker.exe" -ForegroundColor Gray
