# 使用 golang 官方镜像作为基础镜像
FROM golang:latest AS builder

# 设置工作目录
WORKDIR /app

# 拷贝项目文件到工作目录
COPY . .

# 编译项目
RUN go build -o ese *.go

# 使用 Alpine Linux 作为基础镜像
FROM alpine:latest

# 设置工作目录
WORKDIR /app

# 从前一个镜像中拷贝编译好的可执行文件到当前镜像
COPY --from=builder /app/ese .

# 拷贝配置文件
COPY .env.example .env

# 替换配置文件中的数据库和 Redis 配置

# 初始化数据库
RUN ./ese art init

# 手动插入一个真实的 URL 到 pages_00 表中

# 暴露端口
EXPOSE 8080

# 启动应用
CMD ["./ese"]
