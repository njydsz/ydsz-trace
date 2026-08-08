# ========== 前端构建阶段 ==========
FROM node:22-alpine AS web-builder
WORKDIR /web
COPY web/package*.json ./
RUN npm install
COPY web/ ./
RUN npm run build

# ========== 后端构建阶段 ==========
FROM golang:1.26.5-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY logs/go.mod logs/go.sum ./
RUN go mod download
COPY logs/ ./
COPY pkg/ ../pkg/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/ydsz-trace-logs .

# ========== 运行阶段 ==========
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone
WORKDIR /app
RUN mkdir -p /app/temp/logs /app/web/dist
COPY --from=builder /app/ydsz-trace-logs /app/ydsz-trace-logs
COPY logs/conf/app.conf /app/conf/app.conf
COPY --from=web-builder /web/dist /app/web/dist
EXPOSE 2021
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:2021/health || exit 1
ENTRYPOINT ["/app/ydsz-trace-logs"]
