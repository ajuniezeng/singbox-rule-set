#!/bin/bash

set -e -o pipefail

git config --local user.email "github-action@users.noreply.github.com"
git config --local user.name "GitHub Action"
git remote add origin https://github-action:$GITHUB_TOKEN@github.com/ajuniezeng/singbox-rule-set.git
git pull origin master 
git add .
git commit -m "daily update"
git push -f origin master