#!/usr/bin/env bash
# ProxyLens 四平台构建脚本（无外部依赖，只需 Go 1.22+）
# 用法：./build.sh            构建全部平台到 dist/
#       ./build.sh local      只构建当前平台并输出到 build/
set -euo pipefail

cd "$(dirname "$0")"

APP=proxylens
VERSION=${VERSION:-1.0.0}
LDFLAGS="-s -w"

build() {
  local goos=$1 goarch=$2 out=$3
  printf '  → %-16s ' "$goos/$goarch"
  GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 \
    go build -trimpath -ldflags "$LDFLAGS" -o "$out" . && \
    printf '%s\n' "$(du -h "$out" | cut -f1)"
}

if [[ "${1:-all}" == "local" ]]; then
  mkdir -p build
  printf '构建当前平台…\n'
  go build -trimpath -ldflags "$LDFLAGS" -o "build/$APP" .
  echo "完成：build/$APP"
  exit 0
fi

mkdir -p dist
echo "ProxyLens v$VERSION 四平台构建"
build darwin  arm64 "dist/${APP}-${VERSION}-darwin-arm64"
build darwin  amd64 "dist/${APP}-${VERSION}-darwin-amd64"
build windows amd64 "dist/${APP}-${VERSION}-windows-amd64.exe"
build windows arm64 "dist/${APP}-${VERSION}-windows-arm64.exe"
echo
echo "产物在 dist/ 。运行 dist/${APP}-${VERSION}-darwin-arm64 -no-open -port 18770 试试。"
