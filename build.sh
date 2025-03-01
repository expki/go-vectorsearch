#!/bin/bash

if [ ! which wget >/dev/null ] then
    sudo apt update && sudo apt install wget -y
fi
if [ ! -f static/swagger-ui.css ]; then
    wget https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css -O static/swagger-ui.css
fi
if [ ! -f static/swagger-ui-bundle.js ]; then
    wget https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js -O static/swagger-ui-bundle.js
fi
ASSUME_NO_MOVING_GC_UNSAFE_RISK_IT_WITH=go1.24 go build -o gocvd .
ASSUME_NO_MOVING_GC_UNSAFE_RISK_IT_WITH=go1.24 go build -ldflags "-X main.LOCK_THREAD=1" -tags='cuda' -o gocvd-cuda .
