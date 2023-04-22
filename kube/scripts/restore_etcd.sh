#!/bin/bash

# Plan - hard reset and kubelet restart on all local cluster peers. 
# (CIDR RANGE OUT OF BOUNDS+RECONNECT TO CP PODS+CNI RANGES.) 

source .env_poc


ETCD_TEMPORARY_DIR=/tmp/poc/temp_backup
TMP_STAT_DIR=/tmp/poc/kubernetes_static_files

function delete_all_node_port_services(){
	for np_svc in $(sudo -u $KUBECTL_USER kubectl get services --output=jsonpath='{range .items[?(@.spec.type=="NodePort")]}{.metadata.name}{" "}{end}');
	do
		sudo -u $KUBECTL_USER kubectl delete service $np_svc
	done
}

function delete_all_deployments(){
	for np_deploy in $(sudo -u $KUBECTL_USER kubectl get deployments --output=jsonpath='{range .items[]}{.metadata.name}{" "}{end}');
	do
		sudo -u $KUBECTL_USER kubectl delete deployment $np_deploy
	done
}


function redeploy_all_nodeport_services(){
	for np_svc in $(sudo -u $KUBECTL_USER kubectl get service --output=jsonpath='{range .items[?(@.spec.type=="NodePort")]}{.metadata.name}{" "}{end}');
	do
		svc_config=$(sudo -u $KUBECTL_USER kubectl get service $np_svc --output json)
		svc_template=$(echo $svc_config | jq '{ apiVersion: .apiVersion, kind: .kind, metadata: { name: .metadata.name, namespace: .metadata.namespace }, spec: { type: .spec.type, selector: .spec.selector, ports: .spec.ports }}') 
		echo $svc_template > /tmp/poc/placeholder_svc.json
		sudo -u $KUBECTL_USER kubectl delete service $np_svc 
		sudo -u $KUBECTL_USER kubectl create -f /tmp/poc/placeholder_svc.json
		rm /tmp/poc/placholer_svc.json
	done
}
# Reset all pods in the cluster. Used to cleanup backup pod instances not reachable - WILL CAUSE POD LEAKAGE IF POD EXISTS IN CLSTR!
function hard_reset_pods(){
	# Default namespace
	for default_pod in $(sudo -u $KUBECTL_USER kubectl get pods -n default --output=jsonpath='{.items[*].metadata.name}');
	do
		sudo -u $KUBECTL_USER kubectl delete pod --grace-period=0 --force --namespace default $default_pod 
	done
	# CNI Specific - flannel 
	for flannel_pod in $(sudo -u $KUBECTL_USER kubectl get pods -n kube-flannel --output=jsonpath='{.items[*].metadata.name}');
	do
		sudo -u $KUBECTL_USER kubectl delete pod --grace-period=0 --force --namespace kube-flannel $flannel_pod 
	done
	# CP Pods.
	for cp_pod in $(sudo -u $KUBECTL_USER kubectl get pods -n kube-system --output=jsonpath='{.items[*].metadata.name}');
	do
		sudo -u $KUBECTL_USER kubectl delete pod --grace-period=0 --force --namespace kube-system $cp_pod 
	done
}

function get_file_ending_highest_number(){
	local TARGET_DIR=$1
	local MAX=0
	for FILE in "$TARGET_DIR"/*; do
	    if [[ $FILE =~ ^.*_([0-9]+) ]]; then
	        NUM=${BASH_REMATCH[1]}
	        if (( NUM > MAX )); then
	            MAX=$NUM
	        fi
	    fi
	done
	echo $((MAX+1))
}

if [ ! -z $ETCD_SNAPSHOT_PATH ] && [ -a $ETCD_SNAPSHOT_PATH ] && [ -d $ETCD_DATA_DIR ]; then
	ETCDTL_API=3 etcdctl snapshot restore $ETCD_SNAPSHOT_PATH --data-dir $ETCD_TEMPORARY_DIR 
	mkdir -p $TMP_STAT_DIR 
	# Stop api/controller/scheduler.
	mv /etc/kubernetes/manifests/*.yaml $TMP_STAT_DIR/

	# Wait for kube-system servers to shutdown before restoring volumes and starting controll nodes. 
	echo "Await static control plane pods shutdown..."
	sleep 10

	LATEST_RESTORE=$(get_file_ending_highest_number $ETCD_DATA_DIR)
	NEW_DIR_PATH=/var/lib/etcd/member_old_"$LATEST_RESTORE"
	mv $ETCD_DATA_DIR/member $NEW_DIR_PATH 
	mv $ETCD_TEMPORARY_DIR/member $ETCD_DATA_DIR/member
	rmdir $ETCD_TEMPORARY_DIR

	mv $TMP_STAT_DIR/*.yaml /etc/kubernetes/manifests/
	echo "Await static cntrl plane pods startup..."
	sleep 30
	# Hard reset + kubelet restart for cp pods.
	#hard_reset_pods
	# RESET ALL NODE KUBELET SYSTEM.
	sleep 5
	# REDEPLOY ALL NODEPORT SERVICES TO MAKE SURE NO OLD EP OBJECT TO OLD PODS REMAINS.
	redeploy_all_nodeport_services		
	exit 0
fi
exit 1

