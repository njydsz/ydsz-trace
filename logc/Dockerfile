# ========== 构建阶段 ==========
FROM golang:1.26.5-alpine AS builder

# 安装必要工具
RUN apk add --no-cache git

WORKDIR /app

# 复制 go.mod 和 go.sum 先下载依赖（利用缓存层）
COPY logc/go.mod logc/go.sum ./
RUN go mod download

# 复制源码
COPY logc/ ./
COPY pkg/ ../pkg/

# 静态编译
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/ydsz-trace-logc .

# ========== 运行阶段 ==========
FROM alpine:3.19

# 时区与证书
RUN apk add --no-cache ca-certificates tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

WORKDIR /app

# 创建临时目录
RUN mkdir -p /app/temp/logc

# 复制二进制和配置
COPY --from=builder /app/ydsz-trace-logc /app/ydsz-trace-logc
COPY logc/conf/app.conf /app/conf/app.conf

EXPOSE 2020

# 健康检查（K8s liveness）
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:2020/health || exit 1

ENTRYPOINT ["/app/ydsz-trace-logc", "-s", "${YDSZ_LOGS_SERVER:-127.0.0.1:2021}"]
