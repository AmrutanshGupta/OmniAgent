# Activates the isolated router-ml virtual environment in the current PowerShell session
$venvScript = Join-Path $PSScriptRoot "router-ml\.venv\Scripts\Activate.ps1"
if (Test-Path $venvScript) {
    . $venvScript
    Write-Host "[OmniAgent] Activated router-ml virtual environment:" -ForegroundColor Green
    Write-Host "  Python : $((Get-Command python).Source)" -ForegroundColor Cyan
    Write-Host "  Version: $(python --version)" -ForegroundColor Cyan
} else {
    Write-Error "Virtual environment not found at $venvScript"
}
