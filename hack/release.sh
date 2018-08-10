#!/bin/bash

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
BIN_DIR="$SCRIPT_ROOT/bin"
SUPPORTED_OS=(linux darwin windows)
BINARIES=(client server)

for os in ${SUPPORTED_OS[@]}; do
  echo "Building for os $os"
  name="httpt_$os"
  out_dir="$BIN_DIR/$name"
  mkdir "$out_dir"
  for binary in ${BINARIES[@]}; do
    CGO_ENABLED=0 GOOS="$os" GOARCH=amd64 go build -a -o "$out_dir/$binary" "$SCRIPT_ROOT/$binary/main.go"
  done
  tar -zcf "$BIN_DIR/$name.tar.gz" "$out_dir"
done

