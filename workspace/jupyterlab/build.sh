#!/bin/bash

registry="docker-registry-internal.i.brainpp.cn"
repository="brain/ais-workspace"
tag="v0.2"
beta=$(date +%s)

image="$registry/$repository:$tag-$beta"

docker build --no-cache . -f Dockerfile -t "$image"
docker push "$image"
docker rmi "$image"
