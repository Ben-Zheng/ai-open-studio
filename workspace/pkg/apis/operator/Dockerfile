# Build the manager binary
FROM golang:1.17 as builder

WORKDIR /megedu/projects/services/eduapiserver/pkg/apis/workspace

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/

WORKDIR /megedu
# Copy the Go Modules manifests
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod tidy
RUN go mod download

WORKDIR /megedu/projects/services/eduapiserver/pkg/apis/workspace

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /megedu/projects/services/eduapiserver/pkg/apis/workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
