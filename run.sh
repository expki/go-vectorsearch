#!/bin/bash

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
if [ ! -f static/swagger-ui.css ]; then
    wget https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css -O static/swagger-ui.css
fi
if [ ! -f static/swagger-ui-bundle.js ]; then
    wget https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js -O static/swagger-ui-bundle.js
fi
go run .
