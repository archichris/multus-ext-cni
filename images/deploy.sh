#!/bin/bash
#set +e
#docker image prune -f
#docker container prune -f
#docker image rm -f $(docker images | grep multus-hc | awk '{print $3}')
#rm ./bin/* -rf
#set -e 
#./build
#docker build -t multus-hc:$1 . -f Dockerfile.direct
#sed -i "s/image: multus-hc:.*/image: multus-hc:${1}/g" ./images/multus-daemonset.yml
kubectl delete -f ./images/example.yml
kubectl delete -f ./images/mac-vlan-1.yml
kubectl apply -f ./images/multus-daemonset.yml
kubectl -n kube-system delete pod $(kubectl -n kube-system get pod | grep multus| grep Running | awk '{print $1}')
kubectl apply -f ./images/example.yml

