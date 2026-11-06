ARG NODE_IMAGE=node:24-alpine
ARG GO_IMAGE=golang:1.27-alpine
ARG RUNTIME_IMAGE=alpine:3

# Stage 1: 编译前端资产
FROM ${NODE_IMAGE} AS frontend-builder
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: 编译 Go 单一嵌入式二进制 (跨平台目标: linux/amd64)
FROM ${GO_IMAGE} AS backend-builder
WORKDIR /app
COPY server/go.* ./
RUN go env -w GOPROXY="https://goproxy.cn,direct" && go mod download
COPY server/ ./
COPY --from=frontend-builder /web/dist ./assets/web_dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o chronolog .

# Stage 3: 最小安全运行时容器镜像
FROM ${RUNTIME_IMAGE}
RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=backend-builder /app/chronolog /app/chronolog
COPY config/config.example.yaml /app/defaults/config.example.yaml
COPY config/prompts.example.yaml /app/defaults/prompts.example.yaml

VOLUME ["/app/data", "/app/config"]
EXPOSE 8080

ENTRYPOINT ["/app/chronolog"]
