#!/bin/bash
helm install --name etcdcni --set customResources.createEtcdClusterCRD=true --set etcdCluster.enableTLS=true stable/etcd-operator 
