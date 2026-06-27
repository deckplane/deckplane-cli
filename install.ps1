$ErrorActionPreference = "Stop"

$Repo = "deckplane/deckplane-cli"
$BinName = "deckplane.exe"

# Determine Architecture
$Arch = $env:PROCESSOR_ARCHITECTURE
if ($Arch -eq "AMD64") {
    $ArchName = "amd64"
} elseif ($Arch -eq "ARM64") {
    $ArchName = "arm64"
} else {
    Write-Error "Unsupported architecture: $Arch"
    exit 1
}

$Target = "windows-$ArchName"

Write-Host "Detecting latest release..."
$LatestReleaseUrl = "https://api.github.com/repos/$Repo/releases/latest"
try {
    $ReleaseData = Invoke-RestMethod -Uri $LatestReleaseUrl -UseBasicParsing
    $Version = $ReleaseData.tag_name
} catch {
    Write-Error "Failed to fetch latest version from GitHub API. You may have hit the rate limit."
    exit 1
}

if (-not $Version) {
    Write-Error "Could not determine the latest version."
    exit 1
}

$DownloadUrl = "https://github.com/$Repo/releases/download/$Version/deckplane-$Target.exe"
$InstallDir = "$env:USERPROFILE\bin"
$InstallPath = "$InstallDir\$BinName"

if (-not (Test-Path -Path $InstallDir)) {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
}

Write-Host "Downloading deckplane $Version for $Target..."
try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $InstallPath -UseBasicParsing
} catch {
    Write-Error "Failed to download $DownloadUrl"
    exit 1
}

Write-Host "`nSuccessfully installed $BinName to $InstallPath"

# Check if it's in PATH
$EnvPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($EnvPath -notmatch [regex]::Escape($InstallDir)) {
    Write-Host "Adding $InstallDir to your User PATH..."
    $NewPath = "$InstallDir;" + $EnvPath
    [Environment]::SetEnvironmentVariable("PATH", $NewPath, "User")
    $env:PATH = "$InstallDir;" + $env:PATH
    Write-Host "You may need to restart your terminal for the PATH changes to take effect."
}
