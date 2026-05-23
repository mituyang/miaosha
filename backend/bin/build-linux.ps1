$ErrorActionPreference = "Stop"

$backendDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $backendDir

New-Item -ItemType Directory -Force -Path "bin" | Out-Null

$targetArch = if ($env:GOARCH) { $env:GOARCH } else { "amd64" }

$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = $targetArch

go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api
go build -trimpath -ldflags="-s -w" -o bin/worker ./cmd/worker
