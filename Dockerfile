FROM kouleen/golang:1.26 AS builder
LABEL authors="kouleen.china@gmail.com"
# 设置工作目录
WORKDIR /app
# 复制 go.mod 和 go.sum 文件，提前下载依赖
COPY go.mod go.sum ./
# 设置代理，加速下载
RUN go mod download
# 复制整个项目到工作目录
COPY . .
# 构建应用，禁用 CGO
RUN CGO_ENABLED=0 go build -o etcd-admin ./main.go

# 第二阶段：创建轻量级运行时镜像
FROM kouleen/alpine:latest
LABEL authors="Kouleen.china@gmail.com"
RUN apk add --no-cache tzdata && rm -rf /var/cache/apk/*

# 设置工作目录
WORKDIR /app
# 申明容器编码；静态资源启用一年期 immutable 长缓存（资源 URL 带 ?v= 版本号，
# 覆盖为 0 可回退到开发态 no-cache：docker run -e STATIC_CACHE_MAX_AGE=0 ...）
ENV LANG=en_US.UTF-8 \
    LANGUAGE=en_US:en \
    LC_ALL=en_US.UTF-8 \
    TZ=Asia/Shanghai \
    STATIC_CACHE_MAX_AGE=31536000
# 前端静态资源已通过 go:embed 打入二进制，运行时镜像只需单个可执行文件
COPY --from=builder /app/etcd-admin .

# 暴露应用监听的端口，根据实际应用修改
EXPOSE 8080

# 运行应用
CMD ["./etcd-admin"]
