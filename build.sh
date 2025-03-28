#!/bin/bash

if ! command -v gcc &> /dev/null; then
    sudo apt update && sudo apt install gcc -y
fi
if ! command -v wget &> /dev/null; then
    sudo apt update && sudo apt install wget -y
fi
if ! command -v sed &> /dev/null; then
    sudo apt update && sudo apt install sed -y
fi
sed -i "s/go[0-9]\.[0-9]\+/$(go env GOVERSION | sed -E 's/(go[0-9]+\.[0-9]+)\.[0-9]+/\1/')/g" "env/env.go"
if [ ! -f static/api/swagger-ui.css ]; then
    wget https://unpkg.com/swagger-ui-dist@latest/swagger-ui.css -O static/api/swagger-ui.css
fi
if [ ! -f static/api/swagger-ui-bundle.js ]; then
    wget https://unpkg.com/swagger-ui-dist@latest/swagger-ui-bundle.js -O static/api/swagger-ui-bundle.js
fi
mkdir -p build
printf "Building AVX2...\n"
GOAMD64=v3 GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -tags='avx' -o build/vectorsearch .
printf "Building AVX512...\n"
GOAMD64=v4 GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -tags='avx' -o build/vectorsearch-avx512 .
printf "Build completed.\n"
