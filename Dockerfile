# ===== 阶段 1：Vue 前端构建 =====
FROM node:22-alpine AS web-builder

WORKDIR /build

# 复制前端源代码
COPY web/package*.json ./
RUN npm ci

COPY web ./

# 生成优化后的静态资源到 dist/
RUN npm run build

# ===== 阶段 2：Go 后端构建（内嵌 dist） =====
FROM golang:1.22-alpine AS go-builder

WORKDIR /build

# 复制依赖清单
COPY go.mod go.sum ./
RUN go mod download

# 复制源码
COPY . .

# 覆盖 dist：使用阶段 1 构建的最新前端
COPY --from=web-builder /build/dist ./dist

# 编译：禁用 CGO、Linux 目标，注入版本信息
ARG APP_VERSION=v1.8.2
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath -ldflags="-s -w -X github.com/xxl6097/TVBoxWeb/internal/api.AppVersion=${APP_VERSION} -X github.com/xxl6097/TVBoxWeb/internal/api.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%S)" \
    -o /tmp/app ./cmd/tvbox

# ===== 阶段 3：最终运行时镜像 =====
FROM alpine:latest

# 安装 HTTPS 证书支持
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# 仅复制构建产物，不包含源代码和 node_modules
COPY --from=go-builder /tmp/app ./app

# 非 root 运行 + 数据卷（容器重启不丢 store.json）
RUN addgroup -S app && adduser -S app -G app && chown -R app:app /app
USER app
VOLUME ["/app/data"]

# 设置时区
ENV TZ=Asia/Shanghai

# 暴露端口
EXPOSE 8090

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8090/api/health || exit 1

# 启动应用
CMD ["./app"]
