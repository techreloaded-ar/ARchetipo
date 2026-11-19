$ErrorActionPreference = 'Stop'
$distro = 'ArchetipoDocker'
$root = Join-Path $env:RUNNER_TEMP $distro
$rootfs = Join-Path $root 'alpine.tar.gz'
New-Item -ItemType Directory -Path $root | Out-Null

$alpine = 'https://dl-cdn.alpinelinux.org/alpine/v3.23/releases/x86_64/alpine-minirootfs-3.23.8-x86_64.tar.gz'
$sha256 = '56e2810205c17a5dff78cdd7b2c2af4ccdef16f18d765d5bb0f527438b136676'
Invoke-WebRequest -Uri $alpine -OutFile $rootfs
if ((Get-FileHash -Path $rootfs -Algorithm SHA256).Hash -ne $sha256) {
    throw 'Alpine root filesystem checksum differs from the pinned release.'
}

wsl.exe --import $distro (Join-Path $root 'distro') $rootfs --version 2
if ($LASTEXITCODE -ne 0) { throw 'The runner could not start a WSL 2 distribution.' }
wsl.exe --distribution $distro --user root --exec apk add --no-cache docker
if ($LASTEXITCODE -ne 0) { throw 'Docker Engine installation failed in WSL 2.' }

$env:DOCKER_HOST = 'tcp://127.0.0.1:23750'
$dockerLog = Join-Path $root 'docker.log'
$dockerErrorLog = Join-Path $root 'docker-error.log'
$arguments = @('--distribution', $distro, '--user', 'root', '--exec', 'dockerd',
    '--host', $env:DOCKER_HOST, '--tls=false')
$daemon = Start-Process -FilePath wsl.exe -ArgumentList $arguments -PassThru -WindowStyle Hidden `
    -RedirectStandardOutput $dockerLog -RedirectStandardError $dockerErrorLog

$deadline = (Get-Date).AddSeconds(90)
while (-not $daemon.HasExited -and (Get-Date) -lt $deadline) {
    $engine = docker info --format '{{.OSType}}' 2>$null
    if ($LASTEXITCODE -eq 0 -and $engine -eq 'linux') {
        "DOCKER_HOST=$env:DOCKER_HOST" >> $env:GITHUB_ENV
        exit 0
    }
    Start-Sleep -Seconds 2
}

Get-Content -Path $dockerLog, $dockerErrorLog
throw 'The Windows Docker CLI could not reach the Linux engine in WSL 2.'
