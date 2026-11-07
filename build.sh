#!/usr/bin/env bash
# 夜帧影视 · 跨平台打包脚本
# 用法：./build.sh [版本号]   （默认 v1.0.0）
# 产出：release/<版本号>/ 下各平台二进制 + 压缩包
set -euo pipefail

cd "$(dirname "$0")"

VERSION="${1:-v1.0.0}"
RELEASE="release/${VERSION}"
rm -rf "$RELEASE"
mkdir -p "$RELEASE"

echo "==> 1/3 构建前端 (Vite -> server/dist)"
(cd web && npm run build)

echo "==> 2/3 交叉编译各平台二进制（内嵌前端，CGO 关闭）"
# 格式：GOOS/GOARCH:产物名
PLATFORMS=(
  "darwin/arm64:fanren-macos-arm64"
  "darwin/amd64:fanren-macos-amd64"
  "linux/amd64:fanren-linux-amd64"
  "linux/arm64:fanren-linux-arm64"
  "windows/amd64:fanren-windows-amd64.exe"
  "windows/arm64:fanren-windows-arm64.exe"
)

for entry in "${PLATFORMS[@]}"; do
  IFS=':' read -r pair name <<< "$entry"
  IFS='/' read -r os arch <<< "$pair"
  echo "   编译 $os/$arch -> $name"
  (
    cd server
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      go build -trimpath -ldflags "-s -w" -o "../$RELEASE/$name" .
  )
done

echo "==> 3/3 打包压缩"
cd "$RELEASE"
for f in *; do
  [ -f "$f" ] || continue
  if [[ "$f" == *.exe ]]; then
    zip -q "${f%.exe}.zip" "$f"
    rm -f "$f"
  else
    tar -czf "$f.tar.gz" "$f"
    rm -f "$f"
  fi
done

cat > README.txt <<'EOF'
夜帧影视 · 单文件二进制
========================

运行：直接双击或命令行执行，默认监听 http://localhost:8090
首次打开后进「设置 → 播放源 → 订阅」导入订阅链接即可使用。

命令行参数：
  -addr  :8090       监听地址（默认 :8090）
  -data  data        数据目录（配置/历史/收藏等存为 data/store.json）

示例：
  ./fanren-linux-amd64 -addr :9000 -data ./data
  fanren-windows-amd64.exe
EOF

cd ../..

echo ""
echo "==> 完成，产物在 $RELEASE/："
ls -lh "$RELEASE"
