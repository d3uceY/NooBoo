# Builds a throwaway Sims 4 folder in Documents so the app can be exercised
# without touching a real game folder. Re-running it wipes and rebuilds.
#
#   powershell -ExecutionPolicy Bypass -File build/scripts/make-test-folder.ps1

$ErrorActionPreference = 'Stop'

$game = Join-Path $env:USERPROFILE 'Documents\NooBoo Test Folder\The Sims 4'
$root = Split-Path $game -Parent

if (Test-Path -LiteralPath $root) {
  Remove-Item -LiteralPath $root -Recurse -Force
}

function New-Dir([string]$path) {
  New-Item -ItemType Directory -Force -Path $path | Out-Null
}

function New-File([string]$path, [string]$body) {
  New-Dir ([System.IO.Path]::GetDirectoryName($path))
  Set-Content -LiteralPath $path -Value $body -NoNewline
}

# A file with a believable size, so the library shows real-looking numbers.
function New-Blob([string]$path, [int]$kb) {
  New-Dir ([System.IO.Path]::GetDirectoryName($path))
  $bytes = New-Object byte[] ($kb * 1024)
  (New-Object Random 42).NextBytes($bytes)
  [System.IO.File]::WriteAllBytes($path, $bytes)
}

# The six things the app looks for to accept a folder.
New-File (Join-Path $game 'Options.ini') "[Options]`nlastsave=1"
New-File (Join-Path $game 'GameVersion.txt') "1.110.311.1220"
New-Dir  (Join-Path $game 'saves')
New-Dir  (Join-Path $game 'Tray')
New-Dir  (Join-Path $game 'Mods')

# Saves and Tray, so backups have something to snapshot.
1..3 | ForEach-Object { New-Blob (Join-Path $game "saves\Slot_0000000$_.save") 90 }
New-Blob (Join-Path $game 'Tray\household_1.bpi') 40
New-Blob (Join-Path $game 'Tray\room_1.room') 30
New-Blob (Join-Path $game 'Tray\item_1.trayitem') 20

$mods = Join-Path $game 'Mods'

# --- files that must never be listed -----------------------------------------
New-File (Join-Path $mods 'Resource.cfg') "Priority 500`nPackedFile *.package`nPackedFile */*.package"
New-File (Join-Path $mods 'readme.txt') 'How to install these.'
New-Blob (Join-Path $mods '.hidden.package') 4

# --- loose files, one per naming convention players actually use -------------
New-Blob (Join-Path $mods '!MCCC.package') 420                  # priority prefix
New-Blob (Join-Path $mods '[Harrie] Cottage Garden.package') 210 # bracket prefix
New-Blob (Join-Path $mods 'LittleMsSam_AutoLights.package') 26   # Creator_Item
New-Blob (Join-Path $mods 'package_02.package') 18              # generic name
New-Blob (Join-Path $mods 'WickedWhims.package') 780            # starts a pair
New-Blob (Join-Path $mods 'WickedWhims.ts4script') 210          # finishes it
New-Blob (Join-Path $mods 'LittleMsSam_AutoLights.package.off') 3 # switched off

# --- a well-shaped mod folder, the way Install writes them -------------------
New-Blob (Join-Path $mods 'UI Cheats Extension\ui_cheats.package') 74
New-Blob (Join-Path $mods 'UI Cheats Extension\ui_cheats.ts4script') 31

# --- custom content, creator then pack --------------------------------------
New-Blob (Join-Path $mods 'peacemaker-ic\Hamptons\hamptons_bedroom.package') 300
New-Blob (Join-Path $mods 'peacemaker-ic\Hamptons\hamptons_living.package') 280
New-Blob (Join-Path $mods 'ravasheen\Functional Fridge\fridge.package') 45
New-Blob (Join-Path $mods 'Felixandre\Grove\grove.package') 640

# --- a folder nested too deep, and a folder with nothing loadable in it ------
New-Blob (Join-Path $mods 'Downloads\Very Old Stuff\old.package') 55
New-File (Join-Path $mods 'TC Tray Camera\notes.txt') 'no package files here'

# --- an error log whose stack points at one installed mod --------------------
$blamed = Join-Path $mods 'UI Cheats Extension\ui_cheats.ts4script\ui_cheats.py'
$lastException = @"
<?xml version="1.0" ?><root>
<report><version>2</version><sessionid>test</sessionid><type>desync</type><sku>ea.maxis.sims4_64.15.pc</sku><createtime>2026-10-30 09:12:44</createtime><buildsignature>Local.Unknown.Unknown.1.110.311.1220-1.1</buildsignature><categoryid>desync://test/</desync://stentsysconfig/</xsheet><desyncdata>j[mal|g] Error while running the interaction pie menu. (AttributeError: 'NoneType' object has no attribute 'get_pie_menu')&#13;&#10;&#13;&#10;Traceback (most recent call last):&#13;&#10;  File "T:\InGame\Gameplay\Scripts\Core\sims4\utils.py", line 179, in wrapper&#13;&#10;  File "T:\InGame\Gameplay\Scripts\Server\interactions\base\interaction.py", line 5123, in _trigger_interaction_start_event&#13;&#10;  File "$($blamed -replace '\\', '\\')", line 87, in inject_custom_behavior&#13;&#10;AttributeError: 'NoneType' object has no attribute 'get_pie_menu'</desyncdata></report>
</root>
"@
New-File (Join-Path $game 'lastException.txt') $lastException

# --- things to feed the Install screen --------------------------------------
$incoming = Join-Path $root 'downloads-to-install'

Add-Type -AssemblyName System.IO.Compression.FileSystem
$zipPath = Join-Path $incoming 'Some New Mod.zip'
New-Dir $incoming
if (Test-Path -LiteralPath $zipPath) { Remove-Item -LiteralPath $zipPath -Force }
$zip = [System.IO.Compression.ZipFile]::Open($zipPath, 'Create')
foreach ($entry in @('Mods/core.package', 'Mods/extra stuff.package', 'readme.txt')) {
  $e = $zip.CreateEntry($entry)
  $stream = $e.Open()
  $payload = New-Object byte[] 2048
  (New-Object Random 7).NextBytes($payload)
  $stream.Write($payload, 0, $payload.Length)
  $stream.Close()
}
$zip.Dispose()

New-Blob (Join-Path $incoming 'Extra Chairs.package') 64
New-File (Join-Path $incoming 'archive.rar') 'not really rar'

# A whole folder to try the Pick folder button with.
$pack = Join-Path $incoming 'Garden Pack'
New-Blob (Join-Path $pack 'garden_main.package') 120
New-Blob (Join-Path $pack 'Tuning\garden_tuning.package') 40
New-Blob (Join-Path $pack 'Scripts\garden.ts4script') 22
New-File (Join-Path $pack 'install-me.txt') 'not a mod file'

Write-Host ''
Write-Host 'Test folder ready:' -ForegroundColor Green
Write-Host "  $game"
Write-Host ''
Write-Host 'Point NooBoo at it: footer -> Choose folder -> that path.' -ForegroundColor Cyan
Write-Host 'Install sources live next to it:' -ForegroundColor Cyan
Write-Host "  $incoming"
