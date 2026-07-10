$ErrorActionPreference = "Stop"

$Repo = "gvenusleo/atlas"
$InstallDir = if ($env:ATLAS_INSTALL_DIR) { $env:ATLAS_INSTALL_DIR } else { Join-Path $env:USERPROFILE ".local\bin" }

# Parse arguments
$version = ""
for ($i = 0; $i -lt $args.Length; $i++) {
    switch ($args[$i]) {
        "-v" { $version = $args[$i + 1]; $i++ }
        "--version" { $version = $args[$i + 1]; $i++ }
        "-h" { Write-Host "Atlas Installer`n`nUsage: install.ps1 [-v|--version <version>]"; exit 0 }
        "--help" { Write-Host "Atlas Installer`n`nUsage: install.ps1 [-v|--version <version>]"; exit 0 }
        default { Write-Error "Unknown option: $($args[$i])"; exit 1 }
    }
}

# Detect architecture
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { Write-Error "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE"; exit 1 }
}

# Resolve version
if (-not $version) {
    Write-Host "Fetching latest version..."
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $version = $release.tag_name -replace '^v', ''
    if (-not $version) {
        Write-Error "Failed to determine latest version"
        exit 1
    }
}

$artifact = "atlas-windows-$arch-v$version.zip"
$url = "https://github.com/$Repo/releases/download/v$version/$artifact"
$checksumUrl = "https://github.com/$Repo/releases/download/v$version/SHA256SUMS"

Write-Host "Installing Atlas v$version (windows/$arch)"

# Download
$tmpdir = New-Item -ItemType Directory -Force -Path (Join-Path $env:TEMP "atlas-install-$(Get-Random)")
$zipPath = Join-Path $tmpdir.FullName $artifact
$checksumPath = Join-Path $tmpdir.FullName "SHA256SUMS"

Write-Host "Downloading $artifact..."
Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing
Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath -UseBasicParsing

$expectedHash = $null
foreach ($line in Get-Content $checksumPath) {
    $parts = $line -split '\s+', 2
    if ($parts.Count -eq 2 -and $parts[1].TrimStart([char]'*') -eq $artifact) {
        $expectedHash = $parts[0]
        break
    }
}
if (-not $expectedHash) {
    throw "Checksum not found for $artifact"
}
$actualHash = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash
if ($actualHash -ne $expectedHash) {
    throw "Checksum verification failed for $artifact"
}
Write-Host "Checksum verified"

# Extract
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
}
Expand-Archive -Path $zipPath -DestinationPath $InstallDir -Force

# PATH check
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$InstallDir*") {
    Write-Host ""
    Write-Host "Add Atlas to your PATH by running:"
    Write-Host "  [Environment]::SetEnvironmentVariable('Path', `$env:Path + ';$InstallDir', 'User')"
    Write-Host ""
    Write-Host "Then restart your terminal."
} else {
    Write-Host "Atlas is already in your PATH."
}

Write-Host "Atlas installed to $InstallDir\atlas.exe"

# Verify
$atlasExe = Join-Path $InstallDir "atlas.exe"
if (Test-Path $atlasExe) {
    Write-Host "Verification: $(& $atlasExe version)"
}

# Cleanup
Remove-Item -Recurse -Force $tmpdir.FullName -ErrorAction SilentlyContinue
