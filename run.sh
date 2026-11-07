#!/usr/bin/env bash
# 一键构建并启动「鸿蒙影视 · Web 版」
set -e

cd "$(dirname "$0")"

echo "==> 1/3 构建前端 (npm run build)"
(cd web && npm install && npm run build)

echo "==> 2/3 编译后端 (go build)"
(cd server && go build -o tvbox .)

echo "==> 3/3 启动服务 :8090"
cd server && ./tvbox -addr :8090 -data ./data
