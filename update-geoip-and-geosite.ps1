# update database
$geoipDataBaseUrl = "https://github.com/lyc8503/sing-box-rules/releases/latest/download/geoip.db"
$geositeDataBaseUrl = "https://github.com/lyc8503/sing-box-rules/releases/latest/download/geosite.db"

$geoipDataFilePath = ".\geoip.db"
$geositeDataFilePath = ".\geosite.db"

# if the database exist and the update time is less than 1 day, skip update
if (Test-Path $geoipDataFilePath) {
  $geoipDataFile = Get-Item $geoipDataFilePath
  $geoipDataFileLastWriteTime = $geoipDataFile.LastWriteTime
  $geoipDataFileLastWriteTimeSpan = New-TimeSpan -Start $geoipDataFileLastWriteTime -End (Get-Date)
  if ($geoipDataFileLastWriteTimeSpan.Days -lt 1) {
    Write-Host "geoip database is up to date"
  } else {
    Write-Host "geoip database is out of date, update now"
    Invoke-WebRequest -Uri $geoipDataBaseUrl -OutFile $geoipDataFilePath
    Invoke-WebRequest -Uri $geositeDataBaseUrl -OutFile $geositeDataFilePath
  }
}

# extract rules
$geoipAddresses = @(
  "cn",
  "de",
  "facebook",
  "google",
  "netflix",
  "telegram",
  "twitter"
)

$geositeDomains = @(
  "amazon",
  "apple",
  "bilibili",
  "category-ads-all",
  "category-games",
  "category-games@cn",
  "cn",
  "discord",
  "disney",
  "facebook",
  "geolocation-!cn",
  "discord",
  "disney",
  "facebook",
  "geolocation-!cn",
  "github",
  "google",
  "instagram",
  "microsoft",
  "netflix",
  "onedrive",
  "openai",
  "primevideo",
  "steam@cn",
  "telegram",
  "tiktok",
  "tld-!cn",
  "twitch",
  "hbo",
  "twitter",
  "youtube"
)

# export the souce rule-set
foreach ($item in $geoipAddresses) {
  Write-Host "export $item"
  .\sing-box.exe geoip export $item
}

foreach ($item in $geositeDomains) {
  Write-Host "export $item"
  .\sing-box.exe geosite export $item
}

# complile rule-set
$ruleSet = Get-ChildItem -Path .\ -Filter *.json

# make directory for binary files
foreach ($item in $ruleSet) {
  Write-Host "compile $item"
  .\sing-box.exe rule-set compile $item
}

# Move .srs files to bin directory

New-Item -ItemType Directory -Force -Path .\bin
$srsFiles = Get-ChildItem -Path .\ -Filter *.srs

foreach ($item in $srsFiles) {
  Write-Host "move $item to bin"
  Move-Item -Path $item -Destination .\bin -Force
}

# move the source rule-set to rule-set directory
New-Item -ItemType Directory -Force -Path .\rule-set

foreach ($item in $ruleSet) {
  Write-Host "move rule-set source $item to rule-set"
  Move-Item -Path $item -Destination .\rule-set -Force
}

git add .
git commit -m "daily update"
git push