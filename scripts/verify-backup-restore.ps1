# ChronoLog Synthetic Data Backup & Restore Verification (PT-05 / S05)
$ErrorActionPreference = "Stop"
Write-Host ">>> Starting ChronoLog synthetic data backup & restore verification (S05)..." -ForegroundColor Cyan

$RepoRoot = Split-Path -Parent $PSScriptRoot
Set-Location (Join-Path $RepoRoot "server")
$env:GOMODCACHE = Join-Path $env:LOCALAPPDATA 'ChronoLog/go-mod'

go test -v -count=1 -run TestSyntheticDataBackupAndRestore ./internal/storage/...
if ($LASTEXITCODE -eq 0) {
    Write-Host ">>> S05 SUCCESS: Synthetic data backup and new-directory restore verified." -ForegroundColor Green
} else {
    Write-Host ">>> S05 FAILED: Data recovery verification failed." -ForegroundColor Red
    exit 1
}
