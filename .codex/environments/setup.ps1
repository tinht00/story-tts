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

function Resolve-ExistingCommand {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [string[]]$Candidates = @()
    )

    foreach ($candidate in $Candidates) {
        if ($candidate -and (Test-Path $candidate)) {
            return $candidate
        }
    }

    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }

    return $null
}

function Invoke-NativeOrThrow {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Command,
        [string[]]$Arguments = @()
    )

    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Lenh that bai: $Command $($Arguments -join ' ')"
    }
}

function Resolve-PythonCommand {
    $candidates = @(
        @{ Command = "C:\Users\$env:USERNAME\AppData\Local\Programs\Python\Python313\python.exe"; Args = @() },
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

    throw "Khong tim thay Python 3 de tao venv cho tts_service."
}

$npmCommand = Resolve-ExistingCommand -Name "npm.cmd" -Candidates @(
    "C:\Program Files\nodejs\npm.cmd"
)

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
        if (-not $npmCommand) {
            throw "Khong tim thay npm.cmd de cai dependency frontend."
        }
        Invoke-NativeOrThrow -Command $npmCommand -Arguments @("install")
    }
    finally {
        Pop-Location
    }
}

Write-Host "Setup xong."
Write-Host "Backend env: $backendEnv"
Write-Host "Python venv: $venvPython"
Write-Host "edge-tts: $venvEdge"
