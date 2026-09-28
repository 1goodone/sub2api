param(
    [Parameter(Mandatory = $true)]
    [string]$ProtocolRoot,
    [string]$ToSub2Root = $env:TOSUB2_ROOT,
    [string]$TrustedOtpHosts = $env:OPENAI_REAUTH_TRUSTED_OTP_HOSTS,
    [string]$ContainerName = "sub2api",
    [string]$BaseUrl = "http://127.0.0.1:8080"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path $PSScriptRoot -Parent
$workerScript = Join-Path $PSScriptRoot "openai_oauth_reauth_worker.py"
$python = Join-Path $ProtocolRoot ".venv\Scripts\python.exe"

if (-not (Test-Path -LiteralPath $python -PathType Leaf)) {
    throw "Turb virtual environment was not found: $python"
}

$existing = Get-CimInstance Win32_Process | Where-Object {
    $_.CommandLine -and $_.CommandLine.Contains($workerScript, [StringComparison]::OrdinalIgnoreCase)
} | Select-Object -First 1
if ($existing) {
    Write-Output "OpenAI re-auth worker is already running (PID $($existing.ProcessId))."
    exit 0
}

$workerToken = $env:OPENAI_REAUTH_WORKER_TOKEN
if (-not $workerToken) {
    $container = (docker inspect $ContainerName | ConvertFrom-Json)[0]
    $tokenEntry = $container.Config.Env | Where-Object {
        $_.StartsWith("OPENAI_REAUTH_WORKER_TOKEN=", [StringComparison]::Ordinal)
    } | Select-Object -First 1
    if ($tokenEntry) {
        $workerToken = $tokenEntry.Substring("OPENAI_REAUTH_WORKER_TOKEN=".Length)
    }
}
if (-not $workerToken -or $workerToken.Length -lt 32) {
    throw "OPENAI_REAUTH_WORKER_TOKEN is not configured in the environment or $ContainerName container."
}

$logDir = Join-Path $projectRoot "outputs"
[IO.Directory]::CreateDirectory($logDir) | Out-Null
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$outLog = Join-Path $logDir "openai-reauth-worker-$stamp.out.log"
$errLog = Join-Path $logDir "openai-reauth-worker-$stamp.err.log"
$node = (Get-Command node -ErrorAction Stop).Source
$workerEnvironment = @{
    SUB2API_BASE_URL = $BaseUrl
    OPENAI_REAUTH_WORKER_TOKEN = $workerToken
    OPENAI_REAUTH_WORKER_ID = "local-background-worker"
    OPENAI_REAUTH_POLL_SECONDS = "2"
    CODEX_PROTOCOL_ROOT = (Resolve-Path -LiteralPath $ProtocolRoot).Path
    NODE_EXECUTABLE = $node
    PYTHONUNBUFFERED = "1"
}
if ($TrustedOtpHosts) {
    $workerEnvironment.OPENAI_REAUTH_TRUSTED_OTP_HOSTS = $TrustedOtpHosts
}
if ($ToSub2Root) {
    $resolvedToSub2 = (Resolve-Path -LiteralPath $ToSub2Root).Path
    if (-not (Test-Path -LiteralPath (Join-Path $resolvedToSub2 "src\protocol-login.mjs") -PathType Leaf)) {
        throw "toSub2 protocol-login.mjs was not found under: $resolvedToSub2"
    }
    $workerEnvironment.TOSUB2_ROOT = $resolvedToSub2
} else {
    Write-Warning "TOSUB2_ROOT is not configured; password/TOTP re-login tasks will fail."
}

$process = Start-Process -FilePath $python `
    -ArgumentList $workerScript `
    -WorkingDirectory $projectRoot `
    -WindowStyle Hidden `
    -RedirectStandardOutput $outLog `
    -RedirectStandardError $errLog `
    -Environment $workerEnvironment `
    -PassThru

Start-Sleep -Seconds 2
$process.Refresh()
if ($process.HasExited) {
    $startupError = Get-Content -LiteralPath $errLog -Raw -ErrorAction SilentlyContinue
    throw "OpenAI re-auth worker exited during startup (code $($process.ExitCode)): $startupError"
}

Write-Output "OpenAI re-auth worker started (PID $($process.Id))."
Write-Output "Log: $errLog"
