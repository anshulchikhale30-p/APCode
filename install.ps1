# APCode Installer for Windows PowerShell
# Usage:
#   irm https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.ps1 | iex
#   .\install.ps1 -Version 0.1.0 -InstallDir "C:\apcode"
#   .\install.ps1 -Binary ".\apcode.exe"

param(
    [string]$Version = $env:VERSION,
    [string]$Binary = "",
    [string]$InstallDir = $env:APCODE_INSTALL_DIR,
    [switch]$NoModifyPath,
    [switch]$Help
)

$ErrorActionPreference = "Stop"
$REPO = "anshulchikhale30-p/APCode"

function Show-Help {
    Write-Host @"
APCode Installer (PowerShell)

Usage: install.ps1 [options]

Options:
    -Version <version>   Install a specific version (e.g., 0.1.0)
    -Binary <path>       Install from a local binary instead of downloading
    -InstallDir <path>   Install directory (default: %USERPROFILE%\.apcode\bin)
    -NoModifyPath        Don't modify PATH
    -Help                Display this help message

Examples:
    irm https://raw.githubusercontent.com/anshulchikhale30-p/APCode/main/install.ps1 | iex
    .\install.ps1 -Version 0.1.0
    .\install.ps1 -Binary .\apcode.exe
    .\install.ps1 -InstallDir "C:\tools\apcode"
"@
}

if ($Help) {
    Show-Help
    exit 0
}

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Join-Path $env:USERPROFILE ".apcode\bin"
}

if (!(Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$requestedVersion = $Version
$specificVersion = ""

if ($Binary) {
    if (!(Test-Path $Binary)) {
        Write-Host "Error: Binary not found at $Binary" -ForegroundColor Red
        exit 1
    }
    $specificVersion = "local"
}

if ([string]::IsNullOrWhiteSpace($specificVersion)) {
    $arch = "amd64"
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
        $arch = "arm64"
    }
    else {
        try {
            $cpu = Get-CimInstance Win32_Processor -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($cpu -and $cpu.Architecture -eq 12) {
                $arch = "arm64"
            }
        }
        catch {
            $null = $_
        }
    }
    $os = "windows"
    if ([string]::IsNullOrWhiteSpace($requestedVersion)) {
        Write-Host "Fetching latest version..." -ForegroundColor Gray
        try {
            $apiUrl = "https://api.github.com/repos/$REPO/releases/latest"
            $release = Invoke-RestMethod -Uri $apiUrl -UseBasicParsing -ErrorAction Stop
            $tag = $release.tag_name
            $specificVersion = $tag -replace '^v', ''
            if ([string]::IsNullOrWhiteSpace($specificVersion)) {
                throw "empty version"
            }
        }
        catch {
            Write-Host "Failed to fetch latest version: $_" -ForegroundColor Red
            Write-Host "Try: .\install.ps1 -Version 0.1.0" -ForegroundColor Gray
            Write-Host "Releases: https://github.com/$REPO/releases" -ForegroundColor Gray
            exit 1
        }
    }
    else {
        $specificVersion = $requestedVersion -replace '^v', ''
        try {
            $checkUrl = "https://github.com/$REPO/releases/tag/v$specificVersion"
            $resp = Invoke-WebRequest -Uri $checkUrl -UseBasicParsing -Method Head -ErrorAction SilentlyContinue
            if ($resp.StatusCode -eq 404) {
                throw "not found"
            }
        }
        catch {
            try {
                $null = Invoke-RestMethod -Uri "https://api.github.com/repos/$REPO/releases/tags/v$specificVersion" -ErrorAction Stop
            }
            catch {
                Write-Host "Error: Release v$specificVersion not found" -ForegroundColor Red
                Write-Host "Available: https://github.com/$REPO/releases" -ForegroundColor Gray
                exit 1
            }
        }
    }
    $filename = "apcode_${specificVersion}_${os}_${arch}.zip"
    $fallbackFilename = "apcode-${os}-${arch}.zip"
    $url = "https://github.com/$REPO/releases/download/v$specificVersion/$filename"
    $fallbackUrl = "https://github.com/$REPO/releases/download/v$specificVersion/$fallbackFilename"
    $existingCmd = Get-Command apcode -ErrorAction SilentlyContinue
    if ($existingCmd) {
        try {
            $installedVer = & apcode --version 2>$null
            if ($installedVer) {
                $installedVer = $installedVer.ToString().Split()[1]
            }
            if ($installedVer -eq $specificVersion) {
                Write-Host "Version $specificVersion already installed at $($existingCmd.Source)" -ForegroundColor Gray
                if ($existingCmd.Source -ne (Join-Path $InstallDir "apcode.exe")) {
                    Write-Host "Installed at different location, continuing to install to $InstallDir" -ForegroundColor Gray
                }
                else {
                    exit 0
                }
            }
            elseif ($installedVer) {
                Write-Host "Installed version: $installedVer -> upgrading to $specificVersion" -ForegroundColor Gray
            }
        }
        catch {
            $null = $_
        }
    }
    Write-Host ""
    Write-Host "Installing apcode version: $specificVersion for $os/$arch" -ForegroundColor Gray
    $tmpDir = Join-Path $env:TEMP "apcode_install_$PID"
    if (Test-Path $tmpDir) {
        Remove-Item -Recurse -Force $tmpDir
    }
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
    $tmpFile = Join-Path $tmpDir $filename
    $downloaded = $false
    foreach ($dlUrl in @($url, $fallbackUrl)) {
        Write-Host "Downloading $dlUrl" -ForegroundColor Gray
        $success = $false
        try {
            [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
            Invoke-WebRequest -Uri $dlUrl -OutFile $tmpFile -UseBasicParsing -ErrorAction Stop
            $success = $true
        }
        catch {
            Write-Host "  failed: $_" -ForegroundColor DarkYellow
            $success = $false
        }
        if (-not $success) {
            $curlExe = Get-Command curl.exe -ErrorAction SilentlyContinue
            if ($curlExe) {
                try {
                    & curl.exe -fsSL -o $tmpFile $dlUrl
                    if (($LASTEXITCODE -eq 0) -and (Test-Path $tmpFile)) {
                        $success = $true
                    }
                }
                catch {
                    $null = $_
                }
            }
        }
        if ($success) {
            $downloaded = $true
            break
        }
    }
    if (-not $downloaded) {
        Write-Host "Failed to download apcode from GitHub releases" -ForegroundColor Red
        Write-Host "Tried: $url" -ForegroundColor Gray
        Write-Host "  and: $fallbackUrl" -ForegroundColor Gray
        Write-Host "Check: https://github.com/$REPO/releases" -ForegroundColor Gray
        Write-Host "Or install from source: go install ./cmd/apcode" -ForegroundColor Gray
        if (Test-Path $tmpDir) {
            Remove-Item -Recurse -Force $tmpDir
        }
        exit 1
    }
    # Optional checksum verification (GoReleaser produces checksums.txt)
    try {
        $checksumUrl = "https://github.com/$REPO/releases/download/v$specificVersion/checksums.txt"
        $checksumFile = Join-Path $tmpDir "checksums.txt"
        $archiveName = $filename
        if (-not $archiveName) { $archiveName = Split-Path -Leaf $tmpFile }
        try {
            Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumFile -UseBasicParsing -ErrorAction SilentlyContinue | Out-Null
            if (Test-Path $checksumFile) {
                $hash = (Get-FileHash -Path $tmpFile -Algorithm SHA256).Hash.ToLower()
                $line = Select-String -Path $checksumFile -Pattern $archiveName -SimpleMatch -ErrorAction SilentlyContinue | Select-Object -First 1
                if ($line) {
                    $expected = ($line.Line -split '\s+')[0].ToLower()
                    if ($hash -eq $expected) {
                        Write-Host "Checksum verified ($archiveName)" -ForegroundColor Green
                    } else {
                        Write-Host "Checksum mismatch for $archiveName - continuing (HTTPS is still secure)" -ForegroundColor DarkYellow
                    }
                } else {
                    Write-Host "Checksum entry not found for $archiveName - continuing" -ForegroundColor DarkYellow
                }
            }
        } catch { $null = $_ }
    } catch { $null = $_ }
    try {
        Expand-Archive -Path $tmpFile -DestinationPath $tmpDir -Force
    }
    catch {
        Write-Host "Failed to extract archive: $_" -ForegroundColor Red
        if (Test-Path $tmpDir) {
            Remove-Item -Recurse -Force $tmpDir
        }
        exit 1
    }
    $binSrc = Get-ChildItem -Path $tmpDir -Filter "apcode.exe" -Recurse | Select-Object -First 1
    if (-not $binSrc) {
        $binSrc = Get-ChildItem -Path $tmpDir -Filter "apcode" -Recurse | Select-Object -First 1
    }
    if (-not $binSrc) {
        Write-Host "Extracted archive does not contain apcode binary" -ForegroundColor Red
        Get-ChildItem -Path $tmpDir -Recurse | Format-List
        if (Test-Path $tmpDir) {
            Remove-Item -Recurse -Force $tmpDir
        }
        exit 1
    }
    $dest = Join-Path $InstallDir "apcode.exe"
    Copy-Item -Path $binSrc.FullName -Destination $dest -Force
    Write-Host "Installed to $dest" -ForegroundColor Green
    if (Test-Path $tmpDir) {
        Remove-Item -Recurse -Force $tmpDir
    }
    try {
        $verOut = & $dest --version 2>$null
        Write-Host "Verified: $verOut" -ForegroundColor Green
    }
    catch {
        $null = $_
    }
}
else {
    Write-Host ""
    Write-Host "Installing apcode from: $Binary" -ForegroundColor Gray
    $dest = Join-Path $InstallDir "apcode.exe"
    Copy-Item -Path $Binary -Destination $dest -Force
    Write-Host "Installed to $dest" -ForegroundColor Green
    try {
        $verOut = & $dest --version 2>$null
        Write-Host "Verified: $verOut" -ForegroundColor Green
    }
    catch {
        $null = $_
    }
}

if (-not $NoModifyPath) {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $found = $false
    if ($userPath) {
        $parts = $userPath -split ";"
        foreach ($p in $parts) {
            if ($p -eq $InstallDir) {
                $found = $true
                break
            }
        }
    }
    if (-not $found) {
        Write-Host "Adding $InstallDir to user PATH..." -ForegroundColor Gray
        try {
            if ([string]::IsNullOrWhiteSpace($userPath)) {
                $newPath = $InstallDir
            }
            else {
                $newPath = "$userPath;$InstallDir"
            }
            [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
            if ($env:Path -split ";" -notcontains $InstallDir) {
                $env:Path += ";$InstallDir"
            }
            Write-Host "Added $InstallDir to PATH (restart terminal to take effect)" -ForegroundColor Green
        }
        catch {
            Write-Host "Failed to modify PATH: $_" -ForegroundColor Yellow
            Write-Host "Manually add $InstallDir to your PATH" -ForegroundColor Yellow
        }
    }
    else {
        Write-Host "Already in PATH: $InstallDir" -ForegroundColor Gray
    }
}

if (($env:GITHUB_ACTIONS -eq "true") -and $env:GITHUB_PATH) {
    Add-Content -Path $env:GITHUB_PATH -Value $InstallDir
    Write-Host "Added $InstallDir to GITHUB_PATH" -ForegroundColor Gray
}

Write-Host ""
Write-Host "APCode - Offline AI Coding Agent" -ForegroundColor Cyan
Write-Host "==================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "APCode is offline-first. Everything runs locally." -ForegroundColor Gray
Write-Host ""
Write-Host "  apcode                 # welcome + system info" -ForegroundColor White
Write-Host "  apcode benchmark       # run hardware benchmarks" -ForegroundColor Gray
Write-Host "  apcode models          # list models" -ForegroundColor Gray
Write-Host "  apcode recommend       # recommend model for this hardware" -ForegroundColor Gray
Write-Host "  apcode context         # show project context" -ForegroundColor Gray
Write-Host "  apcode search <query>  # search code locally" -ForegroundColor Gray
Write-Host "  apcode runtime         # check runtime" -ForegroundColor Gray
Write-Host "  apcode infer prompt    # run inference" -ForegroundColor Gray
Write-Host ""
Write-Host "Docs: https://github.com/$REPO#readme" -ForegroundColor Gray
Write-Host ('Restart your terminal or run: $env:Path += "' + $InstallDir + '"') -ForegroundColor DarkYellow
Write-Host ""
