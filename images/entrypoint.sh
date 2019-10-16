#!/bin/bash

# Always exit on errors.
set -e

# Run a clean up when we exit if configured to do so.
trap cleanup TERM
function cleanup() {
  if [ "$MULTUS_CLEANUP_CONFIG_ON_EXIT" == "true" ]; then
    CONF=$(
      cat <<-EOF
        {Multus configuration intentionally invalidated to prevent pods from being scheduled.}
EOF
    )
    echo $CONF >$CNI_CONF_DIR/00-multus.conf
    log "Multus configuration intentionally invalidated to prevent pods from being scheduled."
  fi
}

# Set our known directories.
CNI_CONF_DIR="/host/etc/cni/net.d"
CNI_BIN_DIR="/host/opt/cni/bin"
MULTUS_CONF_FILE="/usr/src/multus-cni/images/70-multus.conf"
MULTUS_AUTOCONF_DIR="/host/etc/cni/net.d"
MULTUS_BIN_FILE="/usr/src/multus-cni/bin/multus"
MULTUS_KUBECONFIG_FILE_HOST="/etc/cni/net.d/multus.d/multus.kubeconfig"
MULTUS_NAMESPACE_ISOLATION=false
MULTUS_LOG_LEVEL="error"
MULTUS_LOG_FILE="/tmp/multus.log"
OVERRIDE_NETWORK_NAME=false
MULTUS_CLEANUP_CONFIG_ON_EXIT=false
RESTART_CRIO=false
CRIO_RESTARTED_ONCE=false
RENAME_SOURCE_CONFIG_FILE=false
# host_etcd configuration
EXTEND_FUNCTION=true
DAEMON_BIN_FILE="/usr/src/multus-cni/bin/daemon"
ETCD_CONF_FILE="/tmp/etcd-conf/etcd.conf"
ETCD_FILE_HOST_DIR="/host/etc/cni/net.d/multus.d/etcd"
ETCD_FILE_HOST="/host/etc/cni/net.d/multus.d/etcd/etcd.conf"
SRC_CNI_BIN="/usr/src/multus-cni/cni"
CERTS_CLIENT="/tmp/etcd/certs/client/"
IPAM_BIN_FILE="/usr/src/multus-cni/bin/multus-ipam"
VXLAN_BIN_FILE="/usr/src/multus-cni/bin/multus-vxlan"
EXT_DRIVER_DIR="/usr/src/multus-cni/package"


# Give help text for parameters.
function usage() {
  echo -e "This is an entrypoint script for Multus CNI to overlay its binary and "
  echo -e "configuration into locations in a filesystem. The configuration & binary file "
  echo -e "will be copied to the corresponding configuration directory. When "
  echo -e "'--multus-conf-file=auto' is used, 00-multus.conf will be automatically "
  echo -e "generated from the CNI configuration file of the master plugin (the first file "
  echo -e "in lexicographical order in cni-conf-dir)."
  echo -e ""
  echo -e "./entrypoint.sh"
  echo -e "\t-h --help"
  echo -e "\t--cni-conf-dir=$CNI_CONF_DIR"
  echo -e "\t--cni-bin-dir=$CNI_BIN_DIR"
  echo -e "\t--cni-version=<cniVersion (e.g. 0.3.1)>"
  echo -e "\t--multus-conf-file=$MULTUS_CONF_FILE"
  echo -e "\t--multus-bin-file=$MULTUS_BIN_FILE"
  echo -e "\t--multus-kubeconfig-file-host=$MULTUS_KUBECONFIG_FILE_HOST"
  echo -e "\t--namespace-isolation=$MULTUS_NAMESPACE_ISOLATION"
  echo -e "\t--multus-autoconfig-dir=$MULTUS_AUTOCONF_DIR (used only with --multus-conf-file=auto)"
  echo -e "\t--multus-log-level=$MULTUS_LOG_LEVEL (empty by default, used only with --multus-conf-file=auto)"
  echo -e "\t--multus-log-file=$MULTUS_LOG_FILE (empty by default, used only with --multus-conf-file=auto)"
  echo -e "\t--override-network-name=false (used only with --multus-conf-file=auto)"
  echo -e "\t--cleanup-config-on-exit=false (used only with --multus-conf-file=auto)"
  echo -e "\t--rename-conf-file=false (used only with --multus-conf-file=auto)"
  echo -e "\t--restart-crio=false (restarts CRIO after config file is generated)"
  # multus-ipam Configuration
  echo -e "\t--extend-function=true (enable extend function)"
  echo -e "\t--etcd-conf-file=$ETCD_CONF_FILE"
  echo -e "\t--etcd-file-host=$ETCD_FILE_HOST"
  # echo -e "\t--multus-ipam-bin-file=$MULTUS_IPAM_BIN_FILE"
  # echo -e "\t--multus-vxlan-bin-file=$MULTUS_VXLAN_BIN_FILE"
  # Driver Directory
  echo -e "\t--ext-driver-dir=$EXT_DRIVER_DIR"
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
  --cni-version)
    CNI_VERSION=$VALUE
    ;;
  --cni-conf-dir)
    CNI_CONF_DIR=$VALUE
    ;;
  --cni-bin-dir)
    CNI_BIN_DIR=$VALUE
    ;;
  --multus-conf-file)
    MULTUS_CONF_FILE=$VALUE
    ;;
  --multus-bin-file)
    MULTUS_BIN_FILE=$VALUE
    ;;
  --multus-kubeconfig-file-host)
    MULTUS_KUBECONFIG_FILE_HOST=$VALUE
    ;;
  --namespace-isolation)
    MULTUS_NAMESPACE_ISOLATION=$VALUE
    ;;
  --multus-log-level)
    MULTUS_LOG_LEVEL=$VALUE
    ;;
  --multus-log-file)
    MULTUS_LOG_FILE=$VALUE
    ;;
  --multus-autoconfig-dir)
    MULTUS_AUTOCONF_DIR=$VALUE
    ;;
  --override-network-name)
    OVERRIDE_NETWORK_NAME=$VALUE
    ;;
  --cleanup-config-on-exit)
    MULTUS_CLEANUP_CONFIG_ON_EXIT=$VALUE
    ;;
  --restart-crio)
    RESTART_CRIO=$VALUE
    ;;
  --rename-conf-file)
    RENAME_SOURCE_CONFIG_FILE=$VALUE
    ;;
  --extend-function)
    EXTEND_FUNCTION=$VALUE
    ;;
  --etcd-tls)
    ETCD_TLS=$VALUE
    ;;
  --ext_driver_dir)
    EXT_DRIVER_DIR=$VALUE
    ;;
  *)
    warn "unknown parameter \"$PARAM\""
    ;;
  esac
  shift
done

# Create array of known locations
declare -a arr=($CNI_CONF_DIR $CNI_BIN_DIR $MULTUS_BIN_FILE)
if [ "$MULTUS_CONF_FILE" != "auto" ]; then
  arr+=($MULTUS_CONF_FILE)
fi

# Loop through and verify each location each.
for i in "${arr[@]}"; do
  if [ ! -e "$i" ]; then
    warn "Location $i does not exist"
    exit 1
  fi
done

# Copy files into place and atomically move into final binary name
cp -f $MULTUS_BIN_FILE $CNI_BIN_DIR/_multus
mv -f $CNI_BIN_DIR/_multus $CNI_BIN_DIR/multus
if [ "$MULTUS_CONF_FILE" != "auto" ]; then
  cp -f $MULTUS_CONF_FILE $CNI_CONF_DIR
fi

cp -f $IPAM_BIN_FILE $CNI_BIN_DIR/_multus-ipam
mv -f $CNI_BIN_DIR/_multus-ipam $CNI_BIN_DIR/multus-ipam
cp -f $VXLAN_BIN_FILE $CNI_BIN_DIR/_multus-vxlan
mv -f $CNI_BIN_DIR/_multus-vxlan $CNI_BIN_DIR/multus-vxlan

# Make a multus.d directory (for our kubeconfig)
mkdir -p $CNI_CONF_DIR/multus.d
MULTUS_KUBECONFIG=$CNI_CONF_DIR/multus.d/multus.kubeconfig

# ------------------------------- Generate a "kube-config"
# Inspired by: https://tinyurl.com/y7r2knme
SERVICE_ACCOUNT_PATH=/var/run/secrets/kubernetes.io/serviceaccount
KUBE_CA_FILE=${KUBE_CA_FILE:-$SERVICE_ACCOUNT_PATH/ca.crt}
SERVICEACCOUNT_TOKEN=$(cat $SERVICE_ACCOUNT_PATH/token)
SKIP_TLS_VERIFY=${SKIP_TLS_VERIFY:-false}

# Check if we're running as a k8s pod.
if [ -f "$SERVICE_ACCOUNT_PATH/token" ]; then
  # We're running as a k8d pod - expect some variables.
  if [ -z ${KUBERNETES_SERVICE_HOST} ]; then
    error "KUBERNETES_SERVICE_HOST not set"
    exit 1
  fi
  if [ -z ${KUBERNETES_SERVICE_PORT} ]; then
    error "KUBERNETES_SERVICE_PORT not set"
    exit 1
  fi

  if [ "$SKIP_TLS_VERIFY" == "true" ]; then
    TLS_CFG="insecure-skip-tls-verify: true"
  elif [ -f "$KUBE_CA_FILE" ]; then
    TLS_CFG="certificate-authority-data: $(cat $KUBE_CA_FILE | base64 | tr -d '\n')"
  fi

  # Write a kubeconfig file for the CNI plugin.  Do this
  # to skip TLS verification for now.  We should eventually support
  # writing more complete kubeconfig files. This is only used
  # if the provided CNI network config references it.
  touch $MULTUS_KUBECONFIG
  chmod ${KUBECONFIG_MODE:-600} $MULTUS_KUBECONFIG
  cat >$MULTUS_KUBECONFIG <<EOF
# Kubeconfig file for Multus CNI plugin.
apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: ${KUBERNETES_SERVICE_PROTOCOL:-https}://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}
    $TLS_CFG
users:
- name: multus
  user:
    token: "${SERVICEACCOUNT_TOKEN}"
contexts:
- name: multus-context
  context:
    cluster: local
    user: multus
current-context: multus-context
EOF

else
  warn "Doesn't look like we're running in a kubernetes environment (no serviceaccount token)"
fi

# ---------------------- end Generate a "kube-config".

# ---------------------- Generate a "ectd configuration".
# mkdir -p $MULTUS_VXLAN_HOST

# Copy other missing cni
for cni in $(ls $SRC_CNI_BIN); do
  if [ ! -f $CNI_BIN_DIR/$cni ]; then
    cp $SRC_CNI_BIN/$cni $CNI_BIN_DIR/$cni
  fi
done

# install ipvlan drivers
set +e
modinfo ipvlan
if [ $? == 1 ]; then
    $EXT_DRIVER_DIR/ipvlan/ipvlan.sh update
fi
set -e

# Copy etcd conf
MULTUS_ETCD_DIR=$CNI_CONF_DIR/multus.d/etcd
mkdir -p $MULTUS_ETCD_DIR
cp -f  $ETCD_CONF_FILE $ETCD_FILE_HOST

echo $HOSTNAME > $MULTUS_ETCD_DIR/id

# copy cert, key, ca to etcd directory
if [ -d "$CERTS_CLIENT" ]; then
  mkdir -p $MULTUS_ETCD_DIR/pki
  cp -f $CERTS_CLIENT/* $MULTUS_ETCD_DIR/pki
fi

# ---------------------- end a "ectd configuration".

# ------------------------------- Generate "00-multus.conf"

function generateMultusConf() {
  if [ "$MULTUS_CONF_FILE" == "auto" ]; then
    log "Generating Multus configuration file using files in $MULTUS_AUTOCONF_DIR..."
    found_master=false
    tries=0
    while [ $found_master == false ]; do
      MASTER_PLUGIN="$(ls $MULTUS_AUTOCONF_DIR | grep -E '\.conf(list)?$' | grep -Ev '00-multus\.conf' | head -1)"
      if [ "$MASTER_PLUGIN" == "" ]; then
        if [ $tries -lt 600 ]; then
          if ! (($tries % 5)); then
            log "Attemping to find master plugin configuration, attempt $tries"
          fi
          let "tries+=1"
          # See if the Multus configuration file exists, if it does then clean it up.
          if [ "$MULTUS_CLEANUP_CONFIG_ON_EXIT" == true ] && [ -f "$CNI_CONF_DIR/00-multus.conf" ]; then
            # But first, check if it has the invalidated configuration in it (otherwise we keep doing this over and over.)
            if ! grep -q "invalidated" $CNI_CONF_DIR/00-multus.conf; then
              cleanup
            fi
          fi
          sleep 1
        else
          error "Multus could not be configured: no master plugin was found."
          exit 1
        fi
      else

        found_master=true

        ISOLATION_STRING=""
        if [ "$MULTUS_NAMESPACE_ISOLATION" == true ]; then
          ISOLATION_STRING="\"namespaceIsolation\": true,"
        fi

        LOG_LEVEL_STRING=""
        if [ ! -z "${MULTUS_LOG_LEVEL// /}" ]; then
          case "$MULTUS_LOG_LEVEL" in
          debug) ;;

          error) ;;

          panic) ;;

          verbose) ;;

          *)
            error "Log levels should be one of: debug/verbose/error/panic, did not understand $MULTUS_LOG_LEVEL"
            usage
            exit 1
            ;;
          esac
          LOG_LEVEL_STRING="\"logLevel\": \"$MULTUS_LOG_LEVEL\","
        fi

        LOG_FILE_STRING=""
        if [ ! -z "${MULTUS_LOG_FILE// /}" ]; then
          LOG_FILE_STRING="\"logFile\": \"$MULTUS_LOG_FILE\","
        fi

        CNI_VERSION_STRING=""
        if [ ! -z "${CNI_VERSION// /}" ]; then
          CNI_VERSION_STRING="\"cniVersion\": \"$CNI_VERSION\","
        fi

        if [ "$OVERRIDE_NETWORK_NAME" == "true" ]; then
          MASTER_PLUGIN_NET_NAME="$(cat $MULTUS_AUTOCONF_DIR/$MASTER_PLUGIN |
            python -c 'import json,sys;print json.load(sys.stdin)["name"]')"
        else
          MASTER_PLUGIN_NET_NAME="multus-cni-network"
        fi

        MASTER_PLUGIN_LOCATION=$MULTUS_AUTOCONF_DIR/$MASTER_PLUGIN
        MASTER_PLUGIN_JSON="$(cat $MASTER_PLUGIN_LOCATION)"
        log "Using $MASTER_PLUGIN_LOCATION as a source to generate the Multus configuration"
        CONF=$(
          cat <<-EOF
        {
          $CNI_VERSION_STRING
          "name": "$MASTER_PLUGIN_NET_NAME",
          "type": "multus",
          $ISOLATION_STRING
          $LOG_LEVEL_STRING
          $LOG_FILE_STRING
          "kubeconfig": "$MULTUS_KUBECONFIG_FILE_HOST",
          "delegates": [
            $MASTER_PLUGIN_JSON
          ]
        }
EOF
        )
        echo $CONF >$CNI_CONF_DIR/00-multus.conf
        log "Config file created @ $CNI_CONF_DIR/00-multus.conf"
        echo $CONF

        # If we're not performing the cleanup on exit, we can safely rename the config file.
        if [ "$RENAME_SOURCE_CONFIG_FILE" == true ]; then
          mv ${MULTUS_AUTOCONF_DIR}/${MASTER_PLUGIN} ${MULTUS_AUTOCONF_DIR}/${MASTER_PLUGIN}.old
          log "Original master file moved to ${MULTUS_AUTOCONF_DIR}/${MASTER_PLUGIN}.old"
        fi

        if [ "$RESTART_CRIO" == true ]; then
          # Restart CRIO only once.
          if [ "$CRIO_RESTARTED_ONCE" == false ]; then
            log "Restarting crio"
            systemctl restart crio
            CRIO_RESTARTED_ONCE=true
          fi
        fi
      fi
    done
  fi
}
generateMultusConf

ETCD_CFG_DIR=${ETCD_FILE_HOST_DIR} ${DAEMON_BIN_FILE} &

# ---------------------- end Generate "00-multus.conf".

# Enter either sleep loop, or watch loop...
if [ "$MULTUS_CLEANUP_CONFIG_ON_EXIT" == true ]; then
  log "Entering watch loop..."
  while true; do
    # Check and see if the original master plugin configuration exists...
    if [ ! -f "$MASTER_PLUGIN_LOCATION" ]; then
      log "Master plugin @ $MASTER_PLUGIN_LOCATION has been deleted. Performing cleanup..."
      cleanup
      generateMultusConf
      log "Continuing watch loop after configuration regeneration..."
    fi
    sleep 1
  done
else
  log "Entering sleep (success)..."
  sleep infinity
fi
