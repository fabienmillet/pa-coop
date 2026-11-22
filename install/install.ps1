# pa-coop : installation du mode multijoueur pour Prison Architect (Windows, Steam ou Epic)
#
# Lancement le plus simple : double-cliquer sur install-pa-coop.bat
# ou, dans PowerShell :  irm https://raw.githubusercontent.com/fabienmillet/prison-architect-server/master/install/install.ps1 | iex

$ErrorActionPreference = 'Stop'
$Repo = 'fabienmillet/prison-architect-server'
$Files = @('dbghelp.dll', 'pacoop-server.exe', 'pacoop.txt')
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

function Say($t, $c = 'Gray') { Write-Host $t -ForegroundColor $c }

function Find-Games {
    $found = @()
    # Steam : dossier d'installation + bibliothèques déclarées dans libraryfolders.vdf
    $steam = $null
    foreach ($k in 'HKCU:\Software\Valve\Steam', 'HKLM:\SOFTWARE\WOW6432Node\Valve\Steam', 'HKLM:\SOFTWARE\Valve\Steam') {
        try {
            $p = Get-ItemProperty -Path $k -ErrorAction Stop
            if ($p.SteamPath) { $steam = $p.SteamPath; break }
            if ($p.InstallPath) { $steam = $p.InstallPath; break }
        } catch {}
    }
    if ($steam) {
        $libs = @($steam)
        $vdf = Join-Path $steam 'steamapps\libraryfolders.vdf'
        if (Test-Path $vdf) {
            foreach ($m in [regex]::Matches((Get-Content $vdf -Raw), '"path"\s+"([^"]+)"')) {
                $libs += $m.Groups[1].Value -replace '\\\\', '\'
            }
        }
        foreach ($l in ($libs | Select-Object -Unique)) {
            $d = Join-Path $l 'steamapps\common\Prison Architect'
            if (Test-Path (Join-Path $d 'Prison Architect64.exe')) { $found += [pscustomobject]@{ Name = 'Steam'; Path = $d } }
        }
    }
    # Epic : manifestes du lanceur
    $pd = if ($env:ProgramData) { $env:ProgramData } else { 'C:\ProgramData' }
    $man = Join-Path $pd 'Epic\EpicGamesLauncher\Data\Manifests'
    if (Test-Path $man) {
        foreach ($f in Get-ChildItem $man -Filter *.item) {
            try {
                $j = Get-Content $f.FullName -Raw | ConvertFrom-Json
                if ($j.DisplayName -like '*Prison Architect*' -and (Test-Path (Join-Path $j.InstallLocation 'Prison Architect64.exe'))) {
                    $found += [pscustomobject]@{ Name = 'Epic'; Path = $j.InstallLocation }
                }
            } catch {}
        }
    }
    return $found
}

function Choose($question, $choices) {
    Say ''
    Say $question 'Cyan'
    for ($i = 0; $i -lt $choices.Count; $i++) { Say ("  {0}) {1}" -f ($i + 1), $choices[$i]) 'White' }
    while ($true) {
        $r = Read-Host 'Ton choix'
        if ($r -match '^\d+$' -and [int]$r -ge 1 -and [int]$r -le $choices.Count) { return [int]$r - 1 }
    }
}

Say ''
Say '=== pa-coop : multijoueur Prison Architect sur ton propre serveur ===' 'Green'

# 1. Trouver le jeu
$games = @(Find-Games)
if ($games.Count -eq 0) {
    Say "Je n'ai pas trouvé Prison Architect automatiquement." 'Yellow'
    $p = Read-Host 'Colle le chemin du dossier du jeu (celui qui contient "Prison Architect64.exe")'
    $p = $p.Trim('"', ' ')
    if (-not (Test-Path (Join-Path $p 'Prison Architect64.exe'))) { Say 'Dossier invalide.' 'Red'; Read-Host 'Entrée pour quitter'; exit 1 }
    $game = [pscustomobject]@{ Name = 'manuel'; Path = $p }
} elseif ($games.Count -eq 1) {
    $game = $games[0]
} else {
    $game = $games[(Choose 'Plusieurs installations trouvées :' ($games | ForEach-Object { "$($_.Name) : $($_.Path)" }))]
}
Say ("Jeu trouvé ({0}) : {1}" -f $game.Name, $game.Path) 'Green'

$installed = Test-Path (Join-Path $game.Path 'dbghelp.dll')
$opts = @('Héberger une partie (tes amis te rejoignent)', 'Rejoindre un ami ou un serveur', 'Revenir au multijoueur officiel (désinstaller)')
$mode = Choose 'Que veux-tu faire ?' $opts

# 3. Désinstaller
if ($mode -eq 2) {
    foreach ($f in $Files + @('pacoop.log', 'pacoop-server.log', 'pacoop-selecteur.log', 'pa-coop-LISEZMOI.txt')) {
        $p = Join-Path $game.Path $f
        if (Test-Path $p) { Remove-Item $p -Force }
    }
    Say 'pa-coop désinstallé : le jeu utilise de nouveau le multijoueur officiel.' 'Green'
    Read-Host 'Entrée pour quitter'; exit 0
}

# 2. Télécharger et installer la dernière version
$tmp = Join-Path $env:TEMP ("pa-coop-" + [guid]::NewGuid())
New-Item -ItemType Directory $tmp | Out-Null
try {
    Say 'Téléchargement de la dernière version de pa-coop...'
    $zip = Join-Path $tmp 'pa-coop-windows.zip'
    if ($env:PACOOP_ZIP) { Copy-Item $env:PACOOP_ZIP $zip }   # test : paquet local
    else { Invoke-WebRequest -UseBasicParsing -Uri "https://github.com/$Repo/releases/latest/download/pa-coop-windows.zip" -OutFile $zip }
    Expand-Archive -Path $zip -DestinationPath $tmp -Force
    if (Get-Process -Name 'Prison Architect64' -ErrorAction SilentlyContinue) {
        Say 'Ferme Prison Architect avant de continuer.' 'Yellow'
        Read-Host 'Entrée quand le jeu est fermé'
    }
    Copy-Item (Join-Path $tmp 'dbghelp.dll') $game.Path -Force
    Copy-Item (Join-Path $tmp 'pacoop-server.exe') $game.Path -Force
    Copy-Item (Join-Path $tmp 'LISEZMOI.txt') (Join-Path $game.Path 'pa-coop-LISEZMOI.txt') -Force
} finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

# 4. Configuration
if ($mode -eq 0) {
    $conf = "# pa-coop : heberger (le jeu lance le serveur)`r`nserver=local`r`nupnp=1`r`n"
    Set-Content -Path (Join-Path $game.Path 'pacoop.txt') -Value $conf -Encoding ASCII
    Say ''
    Say 'C''est prêt ! Lance le jeu, charge une prison puis Menu > "Go Online".' 'Green'
    Say 'Au premier lancement, Windows demandera d''autoriser "pacoop-server" : accepte (réseaux privés ET publics).'
    Say 'Tes amis devront mettre ton adresse dans leur installation :'
    try {
        $ip = (Invoke-RestMethod -UseBasicParsing -TimeoutSec 5 'https://api.ipify.org')
        Say ("  - par Internet : {0}  (la box ouvre le port 4533 automatiquement si l'UPnP est activé)" -f $ip) 'White'
    } catch {}
    $lan = $null
    if (Get-Command Get-NetIPAddress -ErrorAction SilentlyContinue) { $lan = (Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object { $_.PrefixOrigin -in 'Dhcp', 'Manual' -and $_.IPAddress -notlike '169.254*' } | Select-Object -First 1).IPAddress }
    if ($lan) { Say ("  - chez toi (même réseau) : {0}" -f $lan) 'White' }
} else {
    $addr = (Read-Host "Adresse de l'hôte ou du serveur (ex. 82.64.12.34 ou pa.monserveur.fr:4533)").Trim()
    if (-not $addr) { Say 'Adresse vide : annulé.' 'Red'; Read-Host 'Entrée pour quitter'; exit 1 }
    $conf = "# pa-coop : rejoindre`r`nserver=$addr`r`n"
    Set-Content -Path (Join-Path $game.Path 'pacoop.txt') -Value $conf -Encoding ASCII
    Say ''
    Say 'C''est prêt ! Lance le jeu puis Menu > "Join Online Game" : la partie de ton ami apparaît dans la liste.' 'Green'
}
Say 'Pour changer de mode ou désinstaller, relance ce script.'
Read-Host 'Entrée pour quitter'
