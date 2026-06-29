$ErrorActionPreference = "Stop"

$Repo = "gvenusleo/atlas"
$InstallDir = if ($env:ATLAS_INSTALL_DIR) { $env:ATLAS_INSTALL_DIR } else { Join-Path $env:USERPROFILE ".local\bin" }

# 解析参数
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

# 检测 ARCH
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { Write-Error "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE"; exit 1 }
}

# 查询版本
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

Write-Host "Installing Atlas v$version (windows/$arch)"

# 下载
$tmpdir = New-Item -ItemType Directory -Force -Path (Join-Path $env:TEMP "atlas-install-$(Get-Random)")
$zipPath = Join-Path $tmpdir.FullName $artifact

Write-Host "Downloading $artifact..."
Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing

# 解压
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
}
Expand-Archive -Path $zipPath -DestinationPath $InstallDir -Force

# PATH 检查
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

# 验证
$atlasExe = Join-Path $InstallDir "atlas.exe"
if (Test-Path $atlasExe) {
    Write-Host "Verification: $(& $atlasExe version)"
}

# 清理
Remove-Item -Recurse -Force $tmpdir.FullName -ErrorAction SilentlyContinue
