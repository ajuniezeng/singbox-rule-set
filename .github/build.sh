#!/bin/bash

set -e -o pipefail

geoipAddresses=("cn" "de" "facebook" "google" "netflix" "telegram" "twitter")
geositeDomains=("amazon" "apple" "bilibili" "category-ads-all" "category-games" "cn" "discord" "disney" "facebook" "geolocation-!cn" "github" "google" "instagram" "microsoft" "netflix" "onedrive" "openai" "primevideo" "telegram" "tiktok" "tld-!cn" "twitch" "hbo" "twitter" "youtube" "threads" "nvidia" "category-httpdns")

for item in "${geoipAddresses[@]}"; do
    ./sing-box geoip export "$item"
done

for item in "${geositeDomains[@]}"; do
    ./sing-box geosite export "$item"
done

mkdir -p bin rule-set
for item in *.json; do
    ./sing-box rule-set compile "$item"
    mv "${item%.json}.srs" bin/
    mv "$item" rule-set/
done