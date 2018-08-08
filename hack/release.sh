#!/bin/bash

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
BIN_DIR=$SCRIPT_ROOT/bin
SUPPORTED_OS=(linux darwin windows)

for os in ${SUPPORTED_OS[@]}; do
  echo "Building for os $os"
  CGO_ENABLED=0 GOOS="$os" GOARCH=amd64 go build -o "$BIN_DIR/httpt_$os" "$SCRIPT_ROOT/main.go"
done

