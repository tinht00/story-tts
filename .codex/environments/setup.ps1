$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$projectRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$projectRoot = Split-Path -Parent $projectRoot
$backendDir = Join-Path $projectRoot "backend"
$frontendDir = Join-Path $projectRoot "frontend"
$ttsDir = Join-Path $projectRoot "tts_service"
$venvDir = Join-Path $projectRoot "data\run\tts-venv"
$venvPython = Join-Path $venvDir "Scripts\python.exe"
$venvEdge = Join-Path $venvDir "Scripts\edge-tts.exe"
$backendEnv = Join-Path $backendDir ".env"
$backendEnvExample = Join-Path $backendDir ".env.example"

function Invoke-NativeOrThrow {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Command,
        [string[]]$Arguments = @()
    )

    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Lệnh thất bại: $Command $($Arguments -join ' ')"
    }
}

function Resolve-PythonCommand {
    $candidates = @(
        @{ Command = "py"; Args = @("-3.13") },
        @{ Command = "py"; Args = @("-3") },
        @{ Command = "python"; Args = @() }
    )

    foreach ($candidate in $candidates) {
        try {
            & $candidate.Command @($candidate.Args + @("--version")) | Out-Null
            return $candidate
        }
        catch {
        }
    }

    throw "Không tìm thấy Python 3 để tạo venv cho tts_service."
}

if (-not (Test-Path $backendEnv) -and (Test-Path $backendEnvExample)) {
    Copy-Item $backendEnvExample $backendEnv
}

if (-not (Test-Path $venvPython)) {
    $python = Resolve-PythonCommand
    Invoke-NativeOrThrow -Command $python.Command -Arguments @($python.Args + @("-m", "venv", $venvDir))
}

Invoke-NativeOrThrow -Command $venvPython -Arguments @("-m", "pip", "install", "--upgrade", "pip")
Invoke-NativeOrThrow -Command $venvPython -Arguments @("-m", "pip", "install", "-r", (Join-Path $ttsDir "requirements.txt"))

if (Test-Path $backendEnv) {
    $envContent = Get-Content $backendEnv -Raw
    $desiredEdgeBinary = "STORY_TTS_EDGE_BINARY=../data/run/tts-venv/Scripts/edge-tts.exe"
    if ($envContent -match "(?m)^STORY_TTS_EDGE_BINARY=") {
        $envContent = [regex]::Replace(
            $envContent,
            "(?m)^STORY_TTS_EDGE_BINARY=.*$",
            $desiredEdgeBinary
        )
    }
    else {
        $envContent = $envContent.TrimEnd() + "`r`n" + $desiredEdgeBinary + "`r`n"
    }
    Set-Content -Path $backendEnv -Value $envContent
}

if (-not (Test-Path (Join-Path $frontendDir "node_modules"))) {
    Push-Location $frontendDir
    try {
        Invoke-NativeOrThrow -Command "npm" -Arguments @("install")
    }
    finally {
        Pop-Location
    }
}

Write-Host "Setup xong."
Write-Host "Backend env: $backendEnv"
Write-Host "Python venv: $venvPython"
Write-Host "edge-tts: $venvEdge"
