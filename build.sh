#!/bin/bash

if ! command -v wget &> /dev/null; then
    sudo apt update && sudo apt install wget -y
fi
if [ ! -f static/swagger-ui.css ]; then
    wget https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css -O static/swagger-ui.css
fi
if [ ! -f static/swagger-ui-bundle.js ]; then
    wget https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js -O static/swagger-ui-bundle.js
fi
go build -o gocvd .
go build -ldflags "-X main.LOCK_THREAD=1" -tags='cuda' -o gocvd-cuda .
