#!/bin/bash
kubectl delete -f examples/vxlan/
rm /var/lib/cni/networks/vxlan1/* -rf
ETCDCTL_API=3 etcdctl --endpoints="192.168.56.201:12379" del multus --prefix
scp ./bin/* 192.168.56.202:/opt/cni/bin/
bin/daemon

