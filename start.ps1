param([switch]$Demo)
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$env:WEBHOOK_SECRET = [guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')
$port = if ($Demo) {8270} else {8260}
$env:ADDR = "127.0.0.1:$port"
$env:PUBLIC_URL = "http://$env:ADDR"
$env:PULSE_ENDPOINT = $env:PUBLIC_URL
$env:FORMS_ADDR = "127.0.0.1:$($port+1)"
$env:SEED_SYNTHETIC = 'false'
$env:DEMO_MODE = 'false'
$env:WEB_DIR = 'web'
$env:STATE_FILE = 'runtime/state.json'
$formsDir = $PSScriptRoot
$env:SURVEY_FILE = (Resolve-Path -LiteralPath 'data/survey.json').Path
if ($Demo) {
    $formsDir = Join-Path $PSScriptRoot ('runtime/rehearsals/' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Force -Path $formsDir | Out-Null
    $env:SURVEY_FILE = (Resolve-Path -LiteralPath 'data/demo-survey.json').Path
    $env:STATE_FILE = Join-Path $formsDir 'state.json'
}
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
    $forms = Start-Process (Join-Path $PSScriptRoot 'forms-service.exe') -WorkingDirectory $formsDir -WindowStyle Hidden -PassThru
    Write-Host "Dashboard: $env:PUBLIC_URL   Forms: http://$env:FORMS_ADDR"
    if ($Demo) { Write-Host "Open $env:PUBLIC_URL/pairs/P6/months/6 for the 60 -> 61 -> 62 demonstration." }
    $pulse.WaitForExit()
} finally {
    foreach ($p in @($forms,$pulse)) {if ($p -and !$p.HasExited) {$p.Kill(); $p.WaitForExit()}}
}
