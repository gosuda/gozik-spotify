param(
  [string]$ServiceName = "GozikSpotifyPlugin",
  [string]$DisplayName = "gozik Spotify Plugin",
  [string]$Port = "50054",
  [string]$WebUIPort = "50055"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$Exe = Join-Path $Root "gozik-spotify.exe"
$Nssm = Join-Path $Root "nssm.exe"

if (!(Test-Path $Exe)) {
  throw "Missing gozik-spotify.exe next to the package root: $Root"
}

if (!(Test-Path $Nssm)) {
  $NssmUrl = "https://nssm.cc/release/nssm-2.24.zip"
  $Zip = Join-Path $env:TEMP "nssm-2.24.zip"
  $Out = Join-Path $env:TEMP "nssm-2.24"
  Invoke-WebRequest -Uri $NssmUrl -OutFile $Zip -UseBasicParsing
  Expand-Archive -Path $Zip -DestinationPath $Out -Force
  Copy-Item (Join-Path $Out "nssm-2.24\win64\nssm.exe") -Destination $Nssm -Force
}

& $Nssm stop $ServiceName 2>$null | Out-Null
& $Nssm remove $ServiceName confirm 2>$null | Out-Null

& $Nssm install $ServiceName $Exe "--port" $Port "--web-ui-port" $WebUIPort "--no-startup-popup" "--register-desktop-entry" "never"
& $Nssm set $ServiceName DisplayName $DisplayName
& $Nssm set $ServiceName Description "Spotify gRPC plugin daemon for gozik"
& $Nssm set $ServiceName AppDirectory $Root
& $Nssm set $ServiceName AppEnvironmentExtra "PATH=$Root;%PATH%" "GOZIK_SPOTIFY_PORT=$Port" "GOZIK_SPOTIFY_WEBUI_PORT=$WebUIPort" "GOZIK_SPOTIFY_NO_POPUP=1"
& $Nssm set $ServiceName Start SERVICE_AUTO_START
& $Nssm start $ServiceName

Write-Host "$DisplayName service installed and started as $ServiceName."
