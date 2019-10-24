#!/bin/bash

# Always exit on errors.
set -e


# Set our known directories.
MULTUS_KUBECONFIG_FILE_HOST="host/etc/cni/net.d/multus.d/multus.kubeconfig"
ETCD_FILE_HOST_DIR="/host/etc/cni/net.d/multus.d/etcd"
MULTUS_LOG_LEVEL="error"
MULTUS_LOG_FILE="/var/log/multus-controller.log"

# Give help text for parameters.
function usage() {
  echo -e "This is an entrypoint script for Multus CNI to overlay its binary and "
  echo -e "\t--multus-kubeconfig-file-host=$MULTUS_KUBECONFIG_FILE_HOST"
  echo -e "\t--etcd-file-host-dir=$ETCD_FILE_HOST_DIR"
  echo -e "\t--multus-log-level=$MULTUS_LOG_LEVEL (empty by default, used only with --multus-conf-file=auto)"
  echo -e "\t--multus-log-file=$MULTUS_LOG_FILE (empty by default, used only with --multus-conf-file=auto)"
}

function log() {
  echo "$(date --iso-8601=seconds) ${1}"
}

function error() {
  log "ERR:  {$1}"
}

function warn() {
  log "WARN: {$1}"
}

# Parse parameters given as arguments to this script.
while [ "$1" != "" ]; do
  PARAM=$(echo $1 | awk -F= '{print $1}')
  VALUE=$(echo $1 | awk -F= '{print $2}')
  case $PARAM in
  -h | --help)
    usage
    exit
    ;;
  --multus-kubeconfig-file-host)
    MULTUS_KUBECONFIG_FILE_HOST=$VALUE
    ;;
  --etcd-file-host-dir)
    ETCD_FILE_HOST_DIR=$VALUE
    ;;
  --multus-log-level)
    MULTUS_LOG_LEVEL=$VALUE
    ;;
  --multus-log-file)
    MULTUS_LOG_FILE=$VALUE
    ;;
  *)
    warn "unknown parameter \"$PARAM\""
    ;;
  esac
  shift
done

KUBE_CONFIG=${MULTUS_KUBECONFIG_FILE_HOST} ETCD_CFG_DIR=${ETCD_FILE_HOST_DIR}  LOG_LEVEL=${MULTUS_LOG_LEVEL} LOG_FILE=${MULTUS_LOG_FILE} /multus-controller