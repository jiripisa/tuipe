# tuipe installer for Windows (PowerShell 5.1 or newer).
#
#   irm https://raw.githubusercontent.com/jiripisa/tuipe/main/install.ps1 | iex
#
# Environment:
#   TUIPE_INSTALL_DIR  target folder (default: $env:LOCALAPPDATA\tuipe\bin)
#   TUIPE_API_BASE     GitHub API base (default: https://api.github.com)
#
# Everything lives in Install-Tuipe, called on the last line, so a truncated
# download cannot run half a script.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Install-Tuipe {
    $repo = 'jiripisa/tuipe'
    $api = if ($env:TUIPE_API_BASE) { $env:TUIPE_API_BASE } else { 'https://api.github.com' }
    $dir = if ($env:TUIPE_INSTALL_DIR) { $env:TUIPE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'tuipe\bin' }

    # Windows PowerShell 5.1 may default to old TLS versions.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    switch ($arch) {
        'AMD64' { $goarch = 'amd64' }
        'ARM64' { $goarch = 'arm64' }
        default { throw "Nepodporovaná architektura: $arch. Podporujeme arm64 a amd64." }
    }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ('tuipe-install-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host 'Zjišťuji nejnovější verzi tuipe...'
        try {
            $release = Invoke-RestMethod -Uri "$api/repos/$repo/releases/latest" -Headers @{ Accept = 'application/vnd.github+json' }
        } catch {
            throw 'Nepodařilo se načíst informace o vydání. Zkontrolujte připojení k internetu.'
        }
        $tag = $release.tag_name
        if (-not $tag) { throw 'V odpovědi chybí číslo verze.' }
        $version = $tag -replace '^v', ''

        $archive = "tuipe_${version}_windows_${goarch}.zip"
        $archiveAsset = $release.assets | Where-Object { $_.name -eq $archive } | Select-Object -First 1
        $sumsAsset = $release.assets | Where-Object { $_.name -eq 'checksums.txt' } | Select-Object -First 1
        if (-not $archiveAsset) { throw "Verze $tag nemá soubor $archive." }
        if (-not $sumsAsset) { throw "Verze $tag nemá soubor checksums.txt." }

        Write-Host "Stahuji tuipe $tag (windows/$goarch)..."
        $archivePath = Join-Path $tmp $archive
        $sumsPath = Join-Path $tmp 'checksums.txt'
        try {
            Invoke-WebRequest -Uri $archiveAsset.browser_download_url -OutFile $archivePath -UseBasicParsing
            Invoke-WebRequest -Uri $sumsAsset.browser_download_url -OutFile $sumsPath -UseBasicParsing
        } catch {
            throw 'Stažení se nezdařilo. Zkontrolujte připojení k internetu.'
        }

        $want = $null
        foreach ($line in Get-Content -Path $sumsPath) {
            $parts = $line -split '\s+'
            if ($parts.Count -eq 2 -and $parts[1].TrimStart('*') -eq $archive) { $want = $parts[0].ToLower(); break }
        }
        if (-not $want) { throw "V checksums.txt chybí kontrolní součet pro $archive." }
        $got = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLower()
        if ($got -ne $want) { throw 'Kontrolní součet nesouhlasí, nic jsem neinstaloval.' }

        $unpacked = Join-Path $tmp 'x'
        Expand-Archive -Path $archivePath -DestinationPath $unpacked
        $exe = Join-Path $unpacked 'tuipe.exe'
        if (-not (Test-Path -LiteralPath $exe)) { throw "Archiv $archive neobsahuje program tuipe.exe." }

        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        $target = Join-Path $dir 'tuipe.exe'
        # Copy next to the target first, then rename: the install is all or nothing.
        $staged = Join-Path $dir '.tuipe.new.exe'
        Copy-Item -LiteralPath $exe -Destination $staged -Force
        Move-Item -LiteralPath $staged -Destination $target -Force
    } finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    # Add the folder to the user's PATH only when it is absent.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = if ($userPath) { $userPath -split ';' | Where-Object { $_ } } else { @() }
    $trimmed = $dir.TrimEnd('\')
    $present = $entries | Where-Object { $_.TrimEnd('\') -ieq $trimmed }
    if (-not $present) {
        $newPath = if ($userPath) { $userPath.TrimEnd(';') + ';' + $dir } else { $dir }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-Host "Složka $dir byla přidána do vaší PATH. Otevřete nový terminál, aby se změna projevila."
    }

    Write-Host 'tuipe nainstalováno. Spusťte: tuipe'
}

try {
    Install-Tuipe
} catch {
    Write-Host "Chyba: $($_.Exception.Message)" -ForegroundColor Red
    if ($PSCommandPath) {
        # Run as a file: report failure to the caller.
        exit 1
    }
    # Under `irm | iex` an `exit` would close the user's terminal, so only
    # flag the failure.
    $global:LASTEXITCODE = 1
}
