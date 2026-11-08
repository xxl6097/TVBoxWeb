# ===== 阶段 1：Go 后端构建 =====
FROM golang:1.21-alpine AS go-builder

WORKDIR /build

# 复制源代码
COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server ./

# 编译：禁用 CGO（便于跨平台，减小体积）、Linux 目标
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /tmp/app ./cmd/main.go

# ===== 阶段 2：Vue 前端构建 =====
FROM node:20-alpine AS web-builder

WORKDIR /build

# 复制前端源代码
COPY web/package*.json ./
RUN npm ci

COPY web ./

# 生成优化后的静态资源
RUN npm run build

# ===== 阶段 3：最终运行时镜像 =====
FROM alpine:latest

# 安装 HTTPS 证书支持
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# 仅复制构建产物，不包含源代码和 node_modules
COPY --from=go-builder /tmp/app ./app
COPY --from=web-builder /build/dist ./dist

# 设置时区
ENV TZ=Asia/Shanghai

# 暴露端口
EXPOSE 8090

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8090/api/health || exit 1

# 启动应用
CMD ["./app"]
