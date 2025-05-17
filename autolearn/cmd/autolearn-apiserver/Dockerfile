FROM golang:1.22 AS builder

#ENV GOPROXY="https://artifact.srdcloud.cn/artifactory/api/go/public-go-virtual,direct"
ENV GOPROXY="https://goproxy.cn,direct"
ENV GO111MODULE=on

# 加一个层缓存go依赖包
COPY go.mod go.sum ./
RUN go mod download

WORKDIR /app
COPY go.mod go.sum ./

# 复制项目
COPY projects/aiservice/autolearn projects/aiservice/autolearn

# 复制项目依赖
COPY . .

WORKDIR /app/projects/aiservice/autolearn/cmd/autolearn-apiserver
RUN CGO_ENABLED=0 go build .

FROM alpine:latest
ENV TZ=Asia/Shanghai
RUN apk add --no-cache tzdata \
  && ln -sf /usr/share/zoneinfo/$TZ /etc/localtime \
  && echo $TZ > /etc/timezone
COPY --from=builder /app/projects/aiservice/autolearn/cmd/autolearn-apiserver/autolearn-apiserver /usr/local/bin/autolearn-apiserver

CMD ["tail", "-f", "dev/null"]
