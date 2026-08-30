#!/usr/bin/env pwsh
# Builds the TournamentStudio frontend and embeds it into the Go binary.
# See README.md "Building from source" for the manual two-step process.
param(
    [string]$Output = "tournamentstudio.exe"
)

$ErrorActionPreference = "Stop"
$RootDir = Split-Path -Parent $MyInvocation.MyCommand.Path

if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    Write-Error "npm not found in PATH"
    exit 1
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "go not found in PATH"
    exit 1
}

Write-Host "==> Building frontend"
Push-Location (Join-Path $RootDir "frontend")
try {
    npm install
    if ($LASTEXITCODE -ne 0) { throw "npm install failed" }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "npm run build failed" }
} finally {
    Pop-Location
}

Write-Host "==> Building Go binary -> $Output"
Push-Location $RootDir
try {
    go build -o $Output ./cmd/tournamentstudio
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
} finally {
    Pop-Location
}

if ([System.IO.Path]::IsPathRooted($Output)) {
    $OutputPath = $Output
} else {
    $OutputPath = Join-Path $RootDir $Output
}
Write-Host "==> Done: $OutputPath"
