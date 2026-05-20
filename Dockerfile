# syntax=docker/dockerfile:1

# ---- Stage 1: 构建前端 ----
# vite 配置把产物输出到 ../internal/web/dist，供 Go 通过 go:embed 内嵌。
FROM node:20-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- Stage 2: 编译 Go（控制面 + Agent 交叉编译） ----
FROM golang:1.26 AS build
ARG VERSION=docker
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 引入上一阶段构建好的前端，供 go:embed 内嵌进控制面二进制
COPY --from=web /app/internal/web/dist ./internal/web/dist
ENV CGO_ENABLED=0
RUN go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/service-edge ./cmd/server \
 && GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/agent-dist/agent_linux_amd64 ./cmd/agent \
 && GOOS=linux GOARCH=arm64 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/agent-dist/agent_linux_arm64 ./cmd/agent

# ---- Stage 3: 运行镜像 ----
# CGO 关闭，二进制为静态文件，可直接在 alpine 上运行。
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/service-edge /app/service-edge
# Agent 二进制目录，控制面通过 --agent-dist 托管，供安装脚本下载
COPY --from=build /out/agent-dist /app/agent-dist
EXPOSE 8443
# 运行时挂载：
#   /etc/service-edge   只读，含 config.yaml、ca.crt、ca.key（不入镜像/不入 git）
#   /var/lib/service-edge 读写，SQLite 数据库持久化目录
ENTRYPOINT ["/app/service-edge", "--config", "/etc/service-edge/config.yaml", "--agent-dist", "/app/agent-dist"]
