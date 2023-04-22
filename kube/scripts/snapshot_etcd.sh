#!/bin/bash
source .env_poc

ETCD_ENDPOINT=$ETCD_ADDR:$ETCD_PORT

if [ -a $ETCD_KEY_LOCATION ] && [ -a $ETCD_CERT_LOCATION ] && [ -a $ETCD_KEY_LOCATION ]; then
	ETCDTL_API=3 etcdctl snapshot save $ETCD_SNAPSHOT_PATH \
		--endpoints=https://$ETCD_ENDPOINT \
		--cacert=$ETCD_CA_CERT_LOCATION \
		--cert=$ETCD_CERT_LOCATION \
		--key=$ETCD_KEY_LOCATION
	exit 0
fi

exit 1
