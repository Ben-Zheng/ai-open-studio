#!/bin/bash

registry="docker-registry-internal.i.brainpp.cn"
repository="brain/ais-workspace-controller"
tag="dev"
beta=$(date +%s)

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager.test main.go
# 请与deploy/release/packages/aiservice/config.yaml保持一致
image="$registry/$repository:$tag-$beta"
docker build --no-cache -f Dockerfile.backup -t "$image" .
docker push "$image"
docker rmi "$image"
