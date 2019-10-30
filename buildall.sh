#!/bin/bash
# use ./buildall.sh <verison>
set -e
./build
docker build -t multus-ext:$1 . -f Dockerfile.direct
docker build -t multus-controller:$1 . -f Dockerfile.controller
sed -i "s#image: \".*multus-ext.*\"#image: \"${DOCKER_REPO}/multus-ext:$1\"#g" ./helm/multus-ext/values.yaml
sed -i "s#image: \".*multus-ext.*\"#image: \"${REMOTE_REPO}/multus-ext:$1\"#g" ./helm/multus-ext/values.remote.yaml
sed -i "s#image: \".*multus-controller.*\"#image: \"${DOCKER_REPO}/multus-controller:$1\"#g" ./helm/multus-ext/values.yaml
sed -i "s#image: \".*multus-controller.*\"#image: \"${REMOTE_REPO}/multus-controller:$1\"#g" ./helm/multus-ext/values.remote.yaml