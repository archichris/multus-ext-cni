#!/bin/bash

currentPath=$(cd "$(dirname "$0")"; pwd)
ipvlan="$currentPath/../ipvlan.sh"

if [ -f "$ipvlan" ];then
    sh "$ipvlan" uninstall
    exit $?
else
    echo "ipvlan.sh not found!"
    exit 1
fi
