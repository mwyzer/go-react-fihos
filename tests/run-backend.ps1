# Runs the FIHOS backend test suite.
#
# - Unit tests: pure Go logic (service, auth, response, middleware). No stack needed.
# - Integration (smoke + regression): hits the running docker-compose stack,
#   uses build tag `integration` so it never runs under plain `go test ./...`.
#
# Usage:
#   pwsh tests/run-backend.ps1            # unit tests only
#   pwsh tests/run-backend.ps1 -Integration   # unit + smoke + regression (stack must be up)
param(
    [switch]$Integration
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location (Join-Path $root 'backend')
try {
    Write-Host "== unit tests ==" -ForegroundColor Cyan
    go test ./internal/service/... ./internal/auth/... ./internal/response/... ./internal/middleware/...
    if ($LastExitCode -ne 0) { throw 'unit tests failed' }

    if ($Integration) {
        Write-Host "`n== integration (smoke + regression) ==" -ForegroundColor Cyan
        $url = if ($env:FIHOS_API_URL) { $env:FIHOS_API_URL } else { 'http://localhost:8081/api/v1' }
        $env:FIHOS_API_URL = $url
        Write-Host "Target: $url"
        go test -tags integration ./itest/ -v
        if ($LastExitCode -ne 0) { throw 'integration tests failed' }
    }
} finally {
    Pop-Location
}