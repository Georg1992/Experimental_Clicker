$ErrorActionPreference = "Stop"

$clickerDir = $PSScriptRoot
$rootDir = Split-Path $clickerDir -Parent
$viiperOut = Join-Path $rootDir "VIIPER\dist\viiper.exe"
$embedDir = Join-Path $clickerDir "gui\embed"
$embedExe = Join-Path $embedDir "viiper.exe"
$clickerOut = Join-Path $rootDir "clicker.exe"
$clickerBuildOut = Join-Path $rootDir "clicker.exe.new"

function Remove-BrokenClickerOutputs {
    param([string]$Root)

    Stop-Process -Name clicker, clicker-console -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 200

    $patterns = @(
        "clicker.exe~",
        "clicker.exe.new",
        "clicker.exe.bak",
        "clicker.exe.old",
        "clicker-*.exe"
    )

    foreach ($pattern in $patterns) {
        Remove-Item (Join-Path $Root $pattern) -Force -ErrorAction SilentlyContinue
    }

    Get-ChildItem -Path $Root -Force -File -ErrorAction SilentlyContinue |
        Where-Object {
            $_.Extension -eq '.exe' -and
            $_.Name -like 'clicker*' -and
            $_.Name -ne 'clicker.exe'
        } |
        Remove-Item -Force -ErrorAction SilentlyContinue
}

Remove-BrokenClickerOutputs -Root $rootDir

New-Item -ItemType Directory -Force $embedDir | Out-Null

if (-not (Test-Path $viiperOut)) {
    Write-Host "Building viiper.exe..." -ForegroundColor Cyan
    Push-Location (Join-Path $rootDir "VIIPER")
    New-Item -ItemType Directory -Force "dist" | Out-Null
    $env:CGO_ENABLED = "0"
    go build -trimpath -o $viiperOut .\cmd\viiper
    if ($LASTEXITCODE -ne 0) { Pop-Location; exit $LASTEXITCODE }
    Pop-Location
}

Copy-Item $viiperOut $embedExe -Force

Push-Location $clickerDir

Write-Host "Downloading Go modules..." -ForegroundColor Cyan
go mod download
if ($LASTEXITCODE -ne 0) { Pop-Location; exit $LASTEXITCODE }

Write-Host "Generating GUI manifest..." -ForegroundColor Cyan
Remove-Item "$clickerDir\gui\*.syso" -Force -ErrorAction SilentlyContinue
Push-Location (Join-Path $clickerDir "gui")
go run github.com/akavel/rsrc@v0.10.2 -manifest app.manifest -o rsrc.syso
if ($LASTEXITCODE -ne 0) { Pop-Location; Pop-Location; exit $LASTEXITCODE }
Pop-Location

Write-Host "Building clicker.exe..." -ForegroundColor Cyan
Remove-Item $clickerBuildOut -Force -ErrorAction SilentlyContinue
go build -trimpath -ldflags="-H windowsgui" -o $clickerBuildOut .\gui
if ($LASTEXITCODE -ne 0) {
    Remove-Item $clickerBuildOut -Force -ErrorAction SilentlyContinue
    Pop-Location
    exit $LASTEXITCODE
}

Remove-Item $clickerOut -Force -ErrorAction SilentlyContinue
Move-Item $clickerBuildOut $clickerOut -Force

Copy-Item (Join-Path $clickerDir "gui\app.manifest") (Join-Path $rootDir "clicker.exe.manifest") -Force

Pop-Location

Remove-BrokenClickerOutputs -Root $rootDir

Write-Host ""
Write-Host "Done. Run:" -ForegroundColor Green
Write-Host "  $clickerOut" -ForegroundColor Yellow
