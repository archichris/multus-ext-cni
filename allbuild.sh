#!/bin/bash
set +e
docker image prune -f
docker container prune -f
docker image rm -f $(docker images | grep multus-hc | awk '{print $3}')
rm ./bin/* -rf 
source /etc/profile
set -e 
./build
docker build -t ${DOCKER_REPO}/multus-hc:$1 . -f Dockerfile.direct
docker tag ${DOCKER_REPO}/multus-hc:$1 ${REMOTE_REPO}/multus-hc:$1
sed -i "s#image: \".*multus-hc.*\"#image: \"${DOCKER_REPO}/multus-hc:$1\"#g" ./helm/multus-hc/values.yaml
sed -i "s#image: \".*multus-hc.*\"#image: \"${REMOTE_REPO}/multus-hc:$1\"#g" ./helm/multus-hc/values.remote.yaml
set +e
mkdir -p ~/helm
rm -rf ~/helm/multus-hc
cp -rf ./helm/multus-hc ~/helm/
mv ~/helm/multus-hc/values.remote.yaml ~/helm/multus-hc/values.yaml
helm package --version $1 ~/helm/multus-hc 
rm -rf ~/helm/multus-hc
mv -f multus-hc*.tgz ~/helm
docker push ${DOCKER_REPO}/multus-hc:$1