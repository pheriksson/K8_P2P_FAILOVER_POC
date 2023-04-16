#!/bin/bash

DEFAULT_ETCD_CA_CERT_LOCATION=/etc/kubernetes/pki/etcd/ca.crt
DEFAULT_ETCD_CERT_LOCATION=/etc/kubernetes/pki/etcd/healthcheck-client.crt
DEFAULT_ETCD_KEY_LOCATION=/etc/kubernetes/pki/etcd/healthcheck-client.key

ETCD_ADDR=$1
ETCD_DEFAULT_PORT=2379
ETCD_SNAPSHOT_NAME=$2
ETCD_ENDPOINT=$ETCD_ADDR:$ETCD_DEFAULT_PORT
echo $1
echo $2

if [ -a $DEFAULT_ETCD_KEY_LOCATION ] && [ -a $DEFAULT_ETCD_CERT_LOCATION ] && [ -a $DEFAULT_ETCD_KEY_LOCATION ]; then
	ETCDTL_API=3 etcdctl --endpoints=https://$ETCD_ENDPOINT snapshot save $ETCD_SNAPSHOT_NAME\
		--cacert=$DEFAULT_ETCD_CA_CERT_LOCATION \
		--cert=$DEFAULT_ETCD_CERT_LOCATION \
		--key=$DEFAULT_ETCD_KEY_LOCATION
	echo "SUCCESS FROM SCRIPT"
	echo $ETCD_SNAPSHOT_NAME
	exit 0
fi
echo "ERROR FROM SCRPT"
exit 1

