# tuipe installer for Windows (PowerShell 5.1 or newer).
#
#   irm https://raw.githubusercontent.com/jiripisa/tuipe/main/install.ps1 | iex
#
# Environment:
#   TUIPE_INSTALL_DIR  target folder (default: $env:LOCALAPPDATA\tuipe\bin)
#   TUIPE_API_BASE     GitHub API base (default: https://api.github.com)
#
# Everything lives in Install-Tuipe, called on the last line, so a truncated
# download cannot run half a script. Preferences are set inside the function
# and the function is removed afterwards, so nothing leaks into the user's
# session under `iex`. This file starts with a UTF-8 BOM so Windows PowerShell
# 5.1 reads the Czech text correctly when run as a file.

function Install-Tuipe {
    $ErrorActionPreference = 'Stop'
    Set-StrictMode -Version Latest

    # Runs $Action; any failure becomes the given Czech message, never raw .NET text.
    function Invoke-Step([string]$Message, [scriptblock]$Action) {
        try { & $Action } catch { throw $Message }
    }

    $repo = 'jiripisa/tuipe'
    $api = if ($env:TUIPE_API_BASE) { $env:TUIPE_API_BASE } else { 'https://api.github.com' }
    $dir = if ($env:TUIPE_INSTALL_DIR) { $env:TUIPE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'tuipe\bin' }

    # Windows PowerShell 5.1 may default to old TLS versions.
    try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch { }

    $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    switch ($arch) {
        'AMD64' { $goarch = 'amd64' }
        'ARM64' { $goarch = 'arm64' }
        default { throw "Nepodporovaná architektura: $arch. Podporujeme arm64 a amd64." }
    }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ('tuipe-install-' + [Guid]::NewGuid().ToString('N'))
    Invoke-Step 'Nepodařilo se vytvořit dočasnou složku.' { [void][IO.Directory]::CreateDirectory($tmp) }
    $staged = $null
    try {
        Write-Host 'Zjišťuji nejnovější verzi tuipe...'
        try {
            $release = Invoke-RestMethod -Uri "$api/repos/$repo/releases/latest" -Headers @{ Accept = 'application/vnd.github+json' } -TimeoutSec 30
        } catch {
            $code = 0
            if ($_.Exception.PSObject.Properties['Response'] -and $_.Exception.Response) {
                $code = [int]$_.Exception.Response.StatusCode
            }
            switch ($code) {
                0 { throw 'Nelze se spojit s GitHubem. Zkontrolujte připojení k internetu a zkuste to znovu.' }
                404 { throw 'Zatím neexistuje žádné vydání ke stažení.' }
                403 { throw 'GitHub dočasně odmítl dotaz (příliš mnoho požadavků). Zkuste to za chvíli.' }
                429 { throw 'GitHub dočasně odmítl dotaz (příliš mnoho požadavků). Zkuste to za chvíli.' }
                default { throw "GitHub odpověděl nečekaně (HTTP $code)." }
            }
        }
        $tag = if ($release.PSObject.Properties['tag_name']) { [string]$release.tag_name } else { '' }
        if (-not $tag) { throw 'GitHub odpověděl nečekaně (chybí číslo verze).' }
        $version = $tag -replace '^v', ''

        $archive = "tuipe_${version}_windows_${goarch}.zip"
        $assets = @($release.assets)
        $archiveAsset = $assets | Where-Object { $_.name -eq $archive } | Select-Object -First 1
        $sumsAsset = $assets | Where-Object { $_.name -eq 'checksums.txt' } | Select-Object -First 1
        if (-not $archiveAsset) { throw "Verze $tag nemá soubor $archive." }
        if (-not $sumsAsset) { throw "Verze $tag nemá soubor checksums.txt." }

        Write-Host "Stahuji tuipe $tag (windows/$goarch)..."
        $archivePath = Join-Path $tmp $archive
        $sumsPath = Join-Path $tmp 'checksums.txt'
        Invoke-Step 'Stažení se nezdařilo. Zkontrolujte připojení k internetu.' {
            Invoke-WebRequest -Uri $archiveAsset.browser_download_url -OutFile $archivePath -UseBasicParsing -TimeoutSec 300
            Invoke-WebRequest -Uri $sumsAsset.browser_download_url -OutFile $sumsPath -UseBasicParsing -TimeoutSec 60
        }

        $want = $null
        $lines = Invoke-Step 'Nepodařilo se přečíst checksums.txt.' { Get-Content -LiteralPath $sumsPath }
        foreach ($line in $lines) {
            $parts = @(([string]$line).Trim() -split '\s+')
            if ($parts.Count -eq 2 -and $parts[1].TrimStart('*') -eq $archive) { $want = $parts[0].ToLower(); break }
        }
        if (-not $want) { throw "V checksums.txt chybí kontrolní součet pro $archive." }
        $got = Invoke-Step 'Nepodařilo se spočítat kontrolní součet.' {
            $sha = [Security.Cryptography.SHA256]::Create()
            $stream = [IO.File]::OpenRead($archivePath)
            try { ([BitConverter]::ToString($sha.ComputeHash($stream)) -replace '-', '').ToLower() }
            finally { $stream.Dispose(); $sha.Dispose() }
        }
        if ($got -ne $want) { throw 'Kontrolní součet nesouhlasí, nic jsem neinstaloval.' }

        $unpacked = Join-Path $tmp 'x'
        Invoke-Step "Archiv $archive je poškozený." {
            Add-Type -AssemblyName System.IO.Compression.FileSystem
            [IO.Compression.ZipFile]::ExtractToDirectory($archivePath, $unpacked)
        }
        $exe = Join-Path $unpacked 'tuipe.exe'
        if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) { throw "Archiv $archive neobsahuje program tuipe.exe." }

        $target = Join-Path $dir 'tuipe.exe'
        $old = $target + '.old'
        if (Test-Path -LiteralPath $target -PathType Container) {
            throw "$target je složka, ne program. Přesuňte ji a spusťte instalaci znovu."
        }
        Invoke-Step "Nepodařilo se vytvořit složku $dir." { [void][IO.Directory]::CreateDirectory($dir) }
        $staged = Join-Path $dir '.tuipe.new.exe'
        Invoke-Step "Nepodařilo se zapsat do $dir." { Copy-Item -LiteralPath $exe -Destination $staged -Force }
        # A running tuipe.exe cannot be overwritten but can be renamed: move it
        # aside first and roll back if the new file cannot take its place.
        $hadOld = Test-Path -LiteralPath $target -PathType Leaf
        try {
            if ($hadOld) {
                Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue
                Move-Item -LiteralPath $target -Destination $old -Force
            }
            try {
                Move-Item -LiteralPath $staged -Destination $target -Force
            } catch {
                if ($hadOld) { Move-Item -LiteralPath $old -Destination $target -Force }
                throw
            }
        } catch {
            throw "Nepodařilo se nainstalovat do $dir. Zavřete spuštěný tuipe a zkuste to znovu."
        }
        $staged = $null
        Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue
    } finally {
        if ($staged) { Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue }
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    # Make plain `tuipe` work: this session now, new terminals via the user PATH.
    $trimmed = $dir.TrimEnd('\')
    $inSession = @(([string]$env:Path) -split ';' | Where-Object { $_ -and $_.TrimEnd('\') -ieq $trimmed })
    if ($inSession.Count -eq 0) {
        $env:Path = ([string]$env:Path).TrimEnd(';') + ';' + $dir
    }
    try {
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $entries = @(([string]$userPath) -split ';' | Where-Object { $_ })
        $present = @($entries | Where-Object { $_.TrimEnd('\') -ieq $trimmed })
        if ($present.Count -eq 0) {
            $newPath = if ($userPath) { $userPath.TrimEnd(';') + ';' + $dir } else { $dir }
            [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
            Write-Host "Složka $dir byla přidána do vaší PATH. Nové terminály ji uvidí, otevřete nový terminál."
        }
    } catch {
        Write-Host "Složku $dir se nepodařilo přidat do PATH. Přidejte ji ručně." -ForegroundColor Yellow
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
} finally {
    Remove-Item -LiteralPath Function:\Install-Tuipe -ErrorAction SilentlyContinue
}
