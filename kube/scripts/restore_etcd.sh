#!/bin/bash

# TODO: Make sure that etcd instance is storing volume at ETCD_DEFAULT_DATA_DIR
ETCD_VOLUME_SNAPSHOT=$1
ETCD_DEFAULT_DATA_DIR=/var/lib/etcd

if [ ! -z $ETCD_VOLUME_SNAPSHOT ] && [ -a $ETCD_VOLUME_SNAPSHOT ] && [ ! -d $ETCD_DEFAULT_DATA_DIR ]
then
	echo "RESTORING ETCD IMAGE"
	ETCDTL_API=3 etcdctl snapshot restore $ETCD_VOLUME_SNAPSHOT --data-dir $ETCD_DEFAULT_DATA_DIR 
	exit 0
else
	echo "FAILED TO ETCD IMAGE"
	echo $ETCD_VOLUME_SNAPSHOT
	exit 1
fi

