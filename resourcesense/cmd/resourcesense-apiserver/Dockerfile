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
COPY projects/aiservice/resourcesense projects/aiservice/resourcesense

# 复制项目依赖
COPY projects/aiservice/components/pkg projects/aiservice/components/pkg
COPY projects/aiservice/auth/pkg projects/aiservice/auth/pkg
COPY projects/aiservice/pkg projects/aiservice/pkg
COPY projects/aiservice/publicservice/pkg projects/aiservice/publicservice/pkg
COPY projects/aiservice/autolearn/pkg projects/aiservice/autolearn/pkg
COPY projects/aiservice/datahub/pkg projects/aiservice/datahub/pkg
COPY projects/aiservice/codehub/pkg projects/aiservice/codehub/pkg
COPY projects/aiservice/modelhub/pkg projects/aiservice/modelhub/pkg
COPY projects/aiservice/algohub/pkg projects/aiservice/algohub/pkg
COPY projects/aiservice/evalhub/pkg projects/aiservice/evalhub/pkg
COPY projects/kubebrain/pkg/auditdb projects/kubebrain/pkg/auditdb

WORKDIR /app/projects/aiservice/resourcesense/cmd/resourcesense-apiserver

# RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build .
RUN CGO_ENABLED=0 go build main.go

FROM alpine:latest
ENV TZ=Asia/Shanghai
RUN apk add --no-cache tzdata \
  && ln -sf /usr/share/zoneinfo/$TZ /etc/localtime \
  && echo $TZ > /etc/timezone
COPY --from=builder /app/projects/aiservice/resourcesense/cmd/resourcesense-apiserver/main  /usr/local/bin/resourcesense-apiserver

CMD ["tail", "-f", "dev/null"]
