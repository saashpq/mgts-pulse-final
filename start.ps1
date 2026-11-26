$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$env:WEBHOOK_SECRET = [guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')
$env:ADDR = '127.0.0.1:8260'
$env:PUBLIC_URL = 'http://127.0.0.1:8260'
$env:PULSE_ENDPOINT = $env:PUBLIC_URL
$env:FORMS_ADDR = '127.0.0.1:8261'
$env:SEED_SYNTHETIC = 'false'
$env:DEMO_MODE = 'false'
$env:WEB_DIR = 'web'
$env:STATE_FILE = 'runtime/state.json'
$first = !(Test-Path $env:STATE_FILE)
$pulse = Start-Process .\pulse.exe -WorkingDirectory $PSScriptRoot -WindowStyle Hidden -PassThru
$forms = $null
try {
    $ok = $false
    for ($i=0; $i -lt 40; $i++) {
        if ($pulse.HasExited) {throw 'Pulse exited. Check the local port.'}
        try {$ok = (Invoke-RestMethod "$env:PUBLIC_URL/health" -TimeoutSec 1).status -eq 'ok'} catch {}
        if ($ok) {break}
        Start-Sleep -Milliseconds 250
    }
    if (!$ok) {throw 'Pulse did not start.'}
    if ($first) {
        foreach ($role in @('intern','mentor')) {
            & curl.exe -fsS -F "role=$role" -F "file=@data/demo-$role.csv" "$env:PUBLIC_URL/api/v1/imports"
            if ($LASTEXITCODE -ne 0) {throw "Sample import failed: $role"}
        }
    }
    $forms = Start-Process .\forms-service.exe -WorkingDirectory $PSScriptRoot -WindowStyle Hidden -PassThru
    Write-Host 'Dashboard: http://127.0.0.1:8260   Forms: http://127.0.0.1:8261'
    $pulse.WaitForExit()
} finally {
    foreach ($p in @($forms,$pulse)) {if ($p -and !$p.HasExited) {$p.Kill(); $p.WaitForExit()}}
}
