[CmdletBinding()]
param(
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Read-DotEnvFile {
    param([string]$Path)

    $values = @{}
    if (-not (Test-Path $Path)) {
        return $values
    }

    foreach ($line in Get-Content $Path) {
        $trimmed = $line.Trim()
        if (-not $trimmed -or $trimmed.StartsWith("#")) {
            continue
        }

        $parts = $trimmed -split "=", 2
        if ($parts.Count -ne 2) {
            continue
        }

        $key = $parts[0].Trim()
        if (-not $key) {
            continue
        }

        $value = $parts[1].Trim().Trim("'`"")
        if (-not $values.ContainsKey($key)) {
            $values[$key] = $value
        }
    }

    return $values
}

function Get-EffectiveStoryTtsEnv {
    param([string]$RepoRoot)

    $effective = @{}
    foreach ($entry in [System.Environment]::GetEnvironmentVariables().GetEnumerator()) {
        $effective[[string]$entry.Key] = [string]$entry.Value
    }

    foreach ($path in @(
        (Join-Path $RepoRoot "backend/.env"),
        (Join-Path $RepoRoot ".env")
    )) {
        $fileValues = Read-DotEnvFile -Path $path
        foreach ($key in $fileValues.Keys) {
            if (-not $effective.ContainsKey($key)) {
                $effective[$key] = $fileValues[$key]
            }
        }
    }

    return $effective
}

function Resolve-PortFromListenAddr {
    param(
        [string]$ListenAddr,
        [int]$Fallback
    )

    if (-not $ListenAddr) {
        return $Fallback
    }

    $match = [regex]::Match($ListenAddr, "(\d+)$")
    if ($match.Success) {
        return [int]$match.Groups[1].Value
    }

    return $Fallback
}

function Get-FrontendDevPort {
    param([string]$FrontendDir)

    $packageJsonPath = Join-Path $FrontendDir "package.json"
    if (-not (Test-Path $packageJsonPath)) {
        return 5174
    }

    $packageJson = Get-Content $packageJsonPath -Raw | ConvertFrom-Json
    $devScript = [string]$packageJson.scripts.dev
    $match = [regex]::Match($devScript, "--port\s+(\d+)")
    if ($match.Success) {
        return [int]$match.Groups[1].Value
    }

    return 5174
}

function Start-DevWindow {
    param(
        [string]$Title,
        [string]$Command,
        [string]$EnvAssignments = "",
        [switch]$DryRun
    )

    $escapedTitle = $Title -replace "'", "''"
    $scriptBlock = @"
$EnvAssignments
`$Host.UI.RawUI.WindowTitle = '$escapedTitle'
$Command
"@

    if ($DryRun) {
        Write-Host ""
        Write-Host "[$Title]" -ForegroundColor Yellow
        Write-Host $scriptBlock
        return
    }

    Start-Process -FilePath "powershell" -ArgumentList @("-NoExit", "-Command", $scriptBlock) | Out-Null
}

function Test-HttpReady {
    param([string]$Url)

    try {
        $null = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2
        return $true
    }
    catch {
        return $false
    }
}

function Wait-HttpReady {
    param(
        [string]$Url,
        [int]$TimeoutSec = 30
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        if (Test-HttpReady -Url $Url) {
            return $true
        }
        Start-Sleep -Milliseconds 750
    }

    return $false
}

$repoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$backendDir = Join-Path $repoRoot "backend"
$frontendDir = Join-Path $repoRoot "frontend"
$ttsPython = Join-Path $repoRoot "data/run/tts-venv/Scripts/python.exe"
$ttsEdgeBinary = Join-Path $repoRoot "data/run/tts-venv/Scripts/edge-tts.exe"

$effectiveEnv = Get-EffectiveStoryTtsEnv -RepoRoot $repoRoot
$backendPort = Resolve-PortFromListenAddr -ListenAddr $effectiveEnv["STORY_TTS_LISTEN_ADDR"] -Fallback 18080
$ttsPort = if ($effectiveEnv.ContainsKey("STORY_TTS_REALTIME_PORT")) { [int]$effectiveEnv["STORY_TTS_REALTIME_PORT"] } else { 8010 }
$ttsHost = if ($effectiveEnv.ContainsKey("STORY_TTS_REALTIME_HOST")) { [string]$effectiveEnv["STORY_TTS_REALTIME_HOST"] } else { "127.0.0.1" }
$frontendPort = Get-FrontendDevPort -FrontendDir $frontendDir

if (-not (Test-Path (Join-Path $backendDir ".env")) -and (Test-Path (Join-Path $backendDir ".env.example"))) {
    Copy-Item (Join-Path $backendDir ".env.example") (Join-Path $backendDir ".env")
}

if (-not (Test-Path $ttsPython)) {
    $ttsPython = "python"
}
if (-not (Test-Path $ttsEdgeBinary) -and $effectiveEnv.ContainsKey("STORY_TTS_EDGE_BINARY")) {
    $configuredEdgeBinary = [string]$effectiveEnv["STORY_TTS_EDGE_BINARY"]
    if ([System.IO.Path]::IsPathRooted($configuredEdgeBinary)) {
        $ttsEdgeBinary = $configuredEdgeBinary
    } else {
        $ttsEdgeBinary = Join-Path $backendDir $configuredEdgeBinary
    }
}

$ttsPythonEscaped = $ttsPython -replace "'", "''"
$ttsEdgeBinaryEscaped = $ttsEdgeBinary -replace "'", "''"
$repoRootEscaped = $repoRoot -replace "'", "''"
$backendDirEscaped = $backendDir -replace "'", "''"
$frontendDirEscaped = $frontendDir -replace "'", "''"
$ttsVoice = if ($effectiveEnv.ContainsKey("STORY_TTS_REALTIME_TTS_VOICE")) { [string]$effectiveEnv["STORY_TTS_REALTIME_TTS_VOICE"] } else { "vi-VN-NamMinhNeural" }
$ttsSpeed = if ($effectiveEnv.ContainsKey("STORY_TTS_REALTIME_TTS_SPEED")) { [string]$effectiveEnv["STORY_TTS_REALTIME_TTS_SPEED"] } else { "0" }
$ttsPitch = if ($effectiveEnv.ContainsKey("STORY_TTS_REALTIME_TTS_PITCH")) { [string]$effectiveEnv["STORY_TTS_REALTIME_TTS_PITCH"] } else { "0" }

$ttsEnvAssignments = @(
    "`$env:STORY_TTS_REALTIME_HOST = '$($ttsHost -replace "'", "''")'"
    "`$env:STORY_TTS_REALTIME_PORT = '$ttsPort'"
    "`$env:STORY_TTS_REALTIME_TTS_VOICE = '$($ttsVoice -replace "'", "''")'"
    "`$env:STORY_TTS_REALTIME_TTS_SPEED = '$($ttsSpeed -replace "'", "''")'"
    "`$env:STORY_TTS_REALTIME_TTS_PITCH = '$($ttsPitch -replace "'", "''")'"
)
if ($ttsEdgeBinary) {
    $ttsEnvAssignments += "`$env:STORY_TTS_EDGE_BINARY = '$ttsEdgeBinaryEscaped'"
}
$ttsEnvAssignments = $ttsEnvAssignments -join [Environment]::NewLine

$ttsCommand = @"
Set-Location '$repoRootEscaped'
& '$ttsPythonEscaped' -m uvicorn tts_service.app:app --host $ttsHost --port $ttsPort
"@

$backendCommand = @"
Set-Location '$backendDirEscaped'
go run ./cmd/api
"@

$frontendCommand = @"
Set-Location '$frontendDirEscaped'
if (-not (Test-Path 'node_modules')) {
    npm install
}
npm run dev
"@

Write-Host ""
Write-Host "Story-TTS local run" -ForegroundColor Cyan
Write-Host "  FE  : http://127.0.0.1:$frontendPort"
Write-Host "  BE  : http://127.0.0.1:$backendPort"
Write-Host "  TTS : http://127.0.0.1:$ttsPort"
Write-Host "  BE health : http://127.0.0.1:$backendPort/health"
Write-Host ""

$backendHealthUrl = "http://127.0.0.1:$backendPort/health"
$ttsHealthUrl = "http://127.0.0.1:$ttsPort/health"
$frontendUrl = "http://127.0.0.1:$frontendPort"

if (-not (Test-HttpReady -Url $ttsHealthUrl)) {
    Start-DevWindow -Title "story-tts | TTS" -Command $ttsCommand -EnvAssignments $ttsEnvAssignments -DryRun:$DryRun
}
else {
    Write-Host "TTS da chay san tai $ttsHealthUrl" -ForegroundColor DarkYellow
}

if (-not (Test-HttpReady -Url $backendHealthUrl)) {
    Start-DevWindow -Title "story-tts | Backend" -Command $backendCommand -DryRun:$DryRun
    if (-not $DryRun) {
        Write-Host "Dang doi backend san sang..." -ForegroundColor DarkYellow
        if (-not (Wait-HttpReady -Url $backendHealthUrl -TimeoutSec 45)) {
            Write-Warning "Backend chua len sau 45s. Kiem tra cua so 'story-tts | Backend' truoc khi mo frontend."
            exit 1
        }
    }
}
else {
    Write-Host "Backend da chay san tai $backendHealthUrl" -ForegroundColor DarkYellow
}

if (-not (Test-HttpReady -Url $frontendUrl)) {
    Start-DevWindow -Title "story-tts | Frontend" -Command $frontendCommand -DryRun:$DryRun
}
else {
    Write-Host "Frontend da chay san tai $frontendUrl" -ForegroundColor DarkYellow
}

if (-not $DryRun) {
    Write-Host "Run xong. Neu frontend vua duoc mo, vao $frontendUrl sau khi Vite build xong."
}
