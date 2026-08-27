# 使用 golang 官方镜像作为基础镜像
FROM golang:1.22-bookworm AS builder

# 设置工作目录
WORKDIR /app

# gojieba 需要 g++ 编译其 CGO 部分
RUN apt-get update && apt-get install -y g++ && rm -rf /var/lib/apt/lists/*

# 拷贝项目文件到工作目录
COPY . .

# 编译项目
RUN go build -o ese *.go

# 使用 Debian slim，避免 CGO/gojieba 二进制在 musl 环境中运行失败
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

# 设置工作目录
WORKDIR /app

# 从前一个镜像中拷贝编译好的可执行文件到当前镜像
COPY --from=builder /app/ese .
COPY --from=builder /app/views ./views
COPY --from=builder /app/dict ./dict

# 拷贝配置文件
COPY .env.example .env

# 暴露端口
EXPOSE 8080

# 启动应用
CMD ["./ese", "all"]
