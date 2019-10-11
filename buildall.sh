#!/bin/bash
# use ./buildall.sh <verison>
set -e
./build
docker build -t multus-ext:$1 . -f Dockerfile.direct
sed -i "s#multus-ext:.*\"#multus-ext:$1\"#g" ./helm/multus-ext/values.yaml
sed -i "s#multus-ext:.*\"#multus-ext:$1\"#g" ./helm/multus-ext/values.remote.yaml