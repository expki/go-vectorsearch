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
if [ ! -f static/swagger-ui.css ]; then
    wget https://unpkg.com/swagger-ui-dist@latest/swagger-ui.css -O static/swagger-ui.css
fi
if [ ! -f static/swagger-ui-bundle.js ]; then
    wget https://unpkg.com/swagger-ui-dist@latest/swagger-ui-bundle.js -O static/swagger-ui-bundle.js
fi
mkdir -p build
GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -tags='avx' -o build/govectorsearch .
if command -v nvcc &> /dev/null; then
    GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -ldflags "-X main.LOCK_THREAD=1" -tags='cuda' -o build/govectorsearch-cuda .
fi
echo "Build completed."
