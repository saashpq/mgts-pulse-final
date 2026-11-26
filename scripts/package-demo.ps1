# Run on Windows from a Git checkout. See scripts/README.md.
param([string]$OutputPath = 'pulse-windows-demo.zip')
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression.FileSystem

$destination = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($OutputPath)
if (Test-Path -LiteralPath $destination) { throw "Output already exists: $destination" }
$project = Split-Path -Parent $PSScriptRoot
$repo = git -C $project rev-parse --show-toplevel
if ($LASTEXITCODE) { throw 'The packager needs a Git checkout.' }
$prefix = git -C $project rev-parse --show-prefix
if ($LASTEXITCODE) { throw 'Cannot locate the project in the source tree.' }
$commit = git -C $repo rev-parse --verify HEAD
if ($LASTEXITCODE) { throw 'Cannot resolve the source revision.' }
$tree = if ($prefix) { "${commit}:$($prefix.TrimEnd('/'))" } else { $commit }
$timestamp = git -C $repo show -s --format=%ct $commit
if ($LASTEXITCODE) { throw 'Cannot read the revision timestamp.' }

$tempRoot = (Get-Item -LiteralPath ([IO.Path]::GetTempPath())).FullName.TrimEnd('\', '/')
$buildName = 'pulse-demo-build-' + [Guid]::NewGuid().ToString('N')
$build = (New-Item -ItemType Directory -Path (Join-Path $tempRoot $buildName)).FullName
$source = Join-Path $build 'src'
$portable = Join-Path $build 'portable'
$buildEnv = @{ GOOS='windows'; GOARCH='amd64'; CGO_ENABLED='0'; GOWORK='off'; GOTOOLCHAIN='local'; VITE_STATIC_DEMO='false'; VITE_API_URL='' }
$previous = @{}
foreach ($name in $buildEnv.Keys) { $previous[$name] = [Environment]::GetEnvironmentVariable($name) }
try {
    foreach ($name in $buildEnv.Keys) { [Environment]::SetEnvironmentVariable($name, $buildEnv[$name]) }
    # The one public/demo fixture is imported by TypeScript tests. Only dist/index.html
    # and dist/assets go into the live dashboard, so it is excluded from the package.
    $inputs = @(
        'go.mod', 'cmd/server', 'internal', 'forms/main.go',
        'forms/public/app.js', 'forms/public/core.js', 'forms/public/index.html',
        'forms/public/intern.json', 'forms/public/mentor.json', 'forms/public/ru.js',
        'forms/public/survey.css', 'forms/public/ui.js', 'forms/public/LICENSE.txt',
        'frontend/src', 'frontend/index.html', 'frontend/package.json', 'frontend/package-lock.json',
        'frontend/tsconfig.json', 'frontend/vite.config.ts', 'frontend/public/demo/pair-P2.json',
        'data/demo-intern.csv', 'data/demo-mentor.csv', 'data/demo-survey.json',
        'licenses/Golos.txt', 'licenses/SurveyJS.txt', 'start.ps1', 'GUIDE.md'
    )
    $sourceZip = Join-Path $build 'source.zip'
    git -C $repo archive --format=zip "--output=$sourceZip" $tree -- @inputs
    if ($LASTEXITCODE) { throw 'Cannot export the committed project inputs.' }
    [IO.Compression.ZipFile]::ExtractToDirectory($sourceZip, $source)
    foreach ($dir in @('data','licenses','web')) {
        [void][IO.Directory]::CreateDirectory((Join-Path $portable $dir))
    }
    Push-Location -LiteralPath $source
    try {
        foreach ($target in @{'pulse.exe'='./cmd/server'; 'forms-service.exe'='./forms'}.GetEnumerator()) {
            go build -trimpath -buildvcs=false -ldflags '-s -w' -o (Join-Path $portable $target.Key) $target.Value
            if ($LASTEXITCODE) { throw "Go build failed: $($target.Key)" }
        }
        Push-Location frontend
        try {
            npm.cmd ci --include=dev --no-audit --no-fund
            if ($LASTEXITCODE) { throw 'Dependency installation failed.' }
            npm.cmd run build
            if ($LASTEXITCODE) { throw 'Dashboard build failed.' }
            Copy-Item -LiteralPath dist/index.html, dist/assets -Destination (Join-Path $portable 'web') -Recurse
        } finally { Pop-Location }
        Copy-Item -LiteralPath data/demo-intern.csv, data/demo-mentor.csv, data/demo-survey.json -Destination (Join-Path $portable 'data')
        # Both modes of the launcher must resolve to the synthetic roster in this package.
        Copy-Item -LiteralPath data/demo-survey.json -Destination (Join-Path $portable 'data/survey.json')
        Copy-Item -LiteralPath licenses/Golos.txt, licenses/SurveyJS.txt -Destination (Join-Path $portable 'licenses')
        Copy-Item -LiteralPath start.ps1, GUIDE.md -Destination $portable
    } finally { Pop-Location }
    $launcher = @(
        '@echo off', 'setlocal', 'set "LLM_MOCK=true"', 'set "HR_TOKEN="',
        'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0start.ps1" -Demo',
        'if errorlevel 1 pause'
    ) -join "`r`n"
    [IO.File]::WriteAllText((Join-Path $portable 'START-DEMO.cmd'), $launcher + "`r`n", [Text.Encoding]::ASCII)

    # Stable entry ordering, timestamps and path separators for repeat builds.
    $fileTime = [DateTimeOffset]::FromUnixTimeSeconds([long]$timestamp)
    $zipPath = Join-Path $build 'demo.zip'
    $archive = [IO.Compression.ZipFile]::Open($zipPath, 'Create')
    try {
        [string[]]$files = [IO.Directory]::GetFiles($portable, '*', 'AllDirectories')
        [Array]::Sort($files, [StringComparer]::Ordinal)
        foreach ($file in $files) {
            $entryName = $file.Substring($portable.Length + 1).Replace('\','/')
            $entry = $archive.CreateEntry($entryName, [IO.Compression.CompressionLevel]::Optimal)
            $entry.LastWriteTime = $fileTime
            $stream = $entry.Open()
            try {
                $inputFile = [IO.File]::OpenRead($file)
                try { $inputFile.CopyTo($stream) } finally { $inputFile.Dispose() }
            } finally { $stream.Dispose() }
        }
    } finally { $archive.Dispose() }
    [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($destination))
    [IO.File]::Copy($zipPath, $destination, $false)
    Write-Host "Built $destination from $commit"
    Get-FileHash -Algorithm SHA256 -LiteralPath $destination
} finally {
    foreach ($name in $previous.Keys) { [Environment]::SetEnvironmentVariable($name, $previous[$name]) }
    $cleanup = (Get-Item -LiteralPath $build).FullName
    if ((Split-Path -Parent $cleanup) -eq $tempRoot -and (Split-Path -Leaf $cleanup) -eq $buildName) {
        # npm can create paths longer than the Windows PowerShell 5.1 provider accepts.
        $longPath = if ($cleanup.StartsWith('\\')) { '\\?\UNC\' + $cleanup.Substring(2) } else { '\\?\' + $cleanup }
        [IO.Directory]::Delete($longPath, $true)
    }
}
