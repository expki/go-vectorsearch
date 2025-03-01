#!/bin/bash

if ! command -v wget &> /dev/null; then
    sudo apt update && sudo apt install wget -y
fi
if [ ! -f static/swagger-ui.css ]; then
    wget https://unpkg.com/swagger-ui-dist@latest/swagger-ui.css -O static/swagger-ui.css
fi
if [ ! -f static/swagger-ui-bundle.js ]; then
    wget https://unpkg.com/swagger-ui-dist@latest/swagger-ui-bundle.js -O static/swagger-ui-bundle.js
fi
GOOS=linux GOARCH=amd64 go build -tags='avx' -o gocvd .
if command -v nvcc &> /dev/null; then
    GOOS=linux GOARCH=amd64 go build -ldflags "-X main.LOCK_THREAD=1" -tags='cuda' -o gocvd-cuda .
fi
echo "Build completed."
