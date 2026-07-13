<#
Installer for mapps (github.com/rus-lan/multiApps).

Usage:
  irm https://raw.githubusercontent.com/rus-lan/multiApps/main/install.ps1 | iex

Env overrides:
  MAPPS_VERSION      pin a release, e.g. v0.1.0 (default: latest)
#>
$ErrorActionPreference = 'Stop'

$Repo = 'rus-lan/multiApps'

function Write-Info($msg) {
	Write-Host $msg
}

# 32-bit PowerShell on a 64-bit OS reports PROCESSOR_ARCHITECTURE as x86;
# PROCESSOR_ARCHITEW6432 carries the real arch in that case.
$arch = $env:PROCESSOR_ARCHITECTURE
if ($env:PROCESSOR_ARCHITEW6432) {
	$arch = $env:PROCESSOR_ARCHITEW6432
}

switch ($arch) {
	'AMD64' { $goArch = 'amd64' }
	'ARM64' { $goArch = 'arm64' }
	default { throw "unsupported architecture: $arch" }
}

$asset = "mapps_windows_$goArch.exe"

$Version = $env:MAPPS_VERSION
if ([string]::IsNullOrEmpty($Version)) {
	$base = "https://github.com/$Repo/releases/latest/download"
} else {
	$base = "https://github.com/$Repo/releases/download/$Version"
}

$binTmp = Join-Path $env:TEMP "$asset.$PID"
$sumsTmp = Join-Path $env:TEMP "checksums.$PID.txt"

try {
	Write-Info "downloading $asset..."
	Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile $binTmp
	Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile $sumsTmp

	Write-Info "verifying checksum..."
	$sumsLine = Select-String -Path $sumsTmp -Pattern ([regex]::Escape($asset) + '$') | Select-Object -First 1
	if (-not $sumsLine) {
		throw "checksum for $asset not found in checksums.txt"
	}
	$expected = ($sumsLine.Line -split '\s+')[0]
	$actual = (Get-FileHash -Algorithm SHA256 -Path $binTmp).Hash

	if ($expected.ToUpperInvariant() -ne $actual.ToUpperInvariant()) {
		Remove-Item -Force $binTmp -ErrorAction SilentlyContinue
		throw ("checksum mismatch for {0}: expected {1}, got {2}" -f $asset, $expected, $actual)
	}

	$InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\mapps'
	New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

	$dest = Join-Path $InstallDir 'mapps.exe'
	Move-Item -Force -Path $binTmp -Destination $dest

	# PATH check/update (user scope).
	$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
	if ($null -eq $userPath) {
		$userPath = ''
	}
	$pathEntries = $userPath -split ';' | Where-Object { $_ -ne '' }
	$alreadyOnPath = $pathEntries | Where-Object { $_.TrimEnd('\') -ieq $InstallDir.TrimEnd('\') }

	if (-not $alreadyOnPath) {
		if ($userPath -and -not $userPath.EndsWith(';')) {
			$newPath = "$userPath;$InstallDir"
		} else {
			$newPath = "$userPath$InstallDir"
		}
		[Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
		Write-Info "added $InstallDir to your user PATH. Open a NEW terminal for this to take effect."
	}

	# Make mapps usable in the current session too, without a new terminal.
	if (($env:Path -split ';') -notcontains $InstallDir) {
		$env:Path = "$env:Path;$InstallDir"
	}

	Write-Info ""
	Write-Info "mapps installed: $(& $dest version)"
	Write-Info "NOTE: mapps.exe runs natively for clone/scaffold, but the Makefile mapps generates needs GNU make and a POSIX shell (Git Bash, MSYS2, or WSL) to run its targets."
} finally {
	Remove-Item -Force $binTmp -ErrorAction SilentlyContinue
	Remove-Item -Force $sumsTmp -ErrorAction SilentlyContinue
}
