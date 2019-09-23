#!/bin/bash
set +e
docker image prune -f
docker container prune -f
docker image rm -f $(docker images | grep multus-hc | awk '{print $3}')
rm ./bin/multus -rf 
source /etc/profile
set -e 
./build
docker build -t ${DOCKER_REPO}/multus-hc:$1 . -f Dockerfile.direct
docker push ${DOCKER_REPO}/multus-hc:$1
sed -i "s/multus-hc:.*\"/multus-hc:${1}\"/g" ./helm/multus-etcd/values.yaml
#kubectl delete -f ./images/example.yml
#kubectl delete -f ./images/mac-vlan-1.yml
#kubectl apply -f ./images/multus-daemonset.yml
#kubectl -n kube-system delete pod $(kubectl -n kube-system get pod | grep multus| grep Running | awk '{print $1}')
#kubectl apply -f ./images/example.yml

