package kube

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
        // https://kubernetes.io/docs/concepts/architecture/nodes/ - Conditions table.
	POD_STATUS_HEALTHY = "Running"
	NODE_HEALTHY_KEY = "Ready" 
	NODE_HEALTHY_VALUE = "True"
	NODE_NETWORK_KEY = "NetworkUnavailable"
	NODE_NETWORK_HEALTHY_KEY = "True"
	TIME_SEC_HEALTH_CHECK = 10
	ETCD_SNAPSHOT_FILENAME = "etcd_snapshot.db"
)
// Need proxy server aswell to send general purpose packets to active kubernetes.
// That way routing wont become a problem.

type KubeClient struct{
	clusterInput chan string
	pocCommands chan string // 
	earlyFailover chan bool
	cluster *kubernetes.Clientset
}

func InitKubeClient(confPath string, failover chan bool) *KubeClient{
	config, err := clientcmd.BuildConfigFromFlags("", confPath)
	if err != nil{
		log.Panic("FAILED TO INIT KUB CLUSTER CLIENT, REASON:", err)
	}
	cluster, err := kubernetes.NewForConfig(config)
	if err != nil{
		log.Panic("FAILED TO INITATE KUB CLUSTER CLIENT. REASON:", err)
	}
	return &KubeClient{make(chan string), make(chan string), failover, cluster};
}




// Used to get node ips for proxy.
// So proxy server will query for node addresses.
// Then on request to proxy, forward request to leader
// Then on leader pick a random node and forward request to that node.
// Complete. and then if failure, simply retry etc. 
type NodeAddress string
type ServicePorts int

// GetNodeAddresses -> have proxy load balance on the random nodes.
// GetServicePorts -> require to have valid service ports to all peers.

// Note: For now only accepting nodePort svc. 
func (k *KubeClient) GetClusterExternalPorts() (error, []ServicePorts){
	services, err := k.cluster.CoreV1().Services("").List(context.TODO(), v1.ListOptions{})
	if err != nil{
		logCluster("FAILED TO GET SERVICES, REASON: "+err.Error())
		return err, []ServicePorts{}
	}
	for _, svc := range services.Items{
		// !ClusterIP -> 
		if (svc.Spec.Type == "NodePort"){
		log.Println(svc.Spec.Ports)
		log.Println(svc.Spec.ExternalTrafficPolicy)

		}
	}
	ports := make([]ServicePorts, len(services.Items))
	return nil, ports 
}


// For proxy, simply fetch addresses and service ports -> return to proxy and loadbalance.
func (k *KubeClient) GetNodeAddresses() (error, NodeAddress){
	nodes ,err := k.cluster.CoreV1().Nodes().List(context.TODO(), v1.ListOptions{})
	// 
	for _, _ = range nodes.Items{
		//for _, cond := range node.Status.Conditions{
		//}
	}
	if err != nil{
		logCluster("FAILED TO GET NODE, REASON: "+err.Error())
		return err, ""
	}
	log.Panic("TODO")
	return nil, ""
}

func (k *KubeClient) checkPodStatus(health chan bool){
	// Will query all pods from ALL namespaces. 
	pods, err := k.cluster.CoreV1().Pods("").List(context.TODO(), v1.ListOptions{})
	if err != nil{
		logCluster("FAILED TO GET PODSm REASON: "+err.Error())
		health<- false
		return
	}
	confirmedHealthy := 0
	for _, pod := range pods.Items{
		if (pod.Status.Phase == POD_STATUS_HEALTHY){
			confirmedHealthy++
		}
	}
	logCluster(fmt.Sprintf("Healthy pods: %d/%d",confirmedHealthy, len(pods.Items)))
	if confirmedHealthy == len(pods.Items){health<- true}else{health<- false}
}

func (k *KubeClient) checkNodeStatus(health chan  bool){
	nodes ,err := k.cluster.CoreV1().Nodes().List(context.TODO(), v1.ListOptions{})
	if err != nil{
		logCluster("FAILED TO GET NODES, REASON: "+err.Error())
		health<- false
		return
	}

	confirmedHealthy := 0
	for _, node := range nodes.Items{
		for _, cond := range node.Status.Conditions{
			// NOTE: for now only checking 'Ready' type 
			if cond.Type == NODE_HEALTHY_KEY && cond.Status == NODE_HEALTHY_VALUE {
				confirmedHealthy++;
				break;
			}
		}
	}
	logCluster(fmt.Sprintf("Healthy nodes: %d/%d",confirmedHealthy, len(nodes.Items)))
	if confirmedHealthy == len(nodes.Items){health<- true} else {health<- false}
}
func (k *KubeClient) Start(){
	// If etcd instance exists start from that, else start from init yaml.
	log.Println("TODO: Start cluster operations.")
	// Have channel to indicate failover. 
	// Stop conditions -> health checks start to fail -> init early failover and wait.
	// Or abort from poc signaling new leader enroled  (self is not leader). 

	err := k.startCluster()
	//TODO
	//k.GetClusterExternalPorts()
	//k.GetNodeAddresses()
	if err != nil{
		log.Panic("CANNOT START CLUSTER, REASON: ",err)
	}
	os.Exit(0)
	abort := make(chan bool)
	clusterUnhealthy := k.startHealthChecks(abort)
	for{
		select{
		case <-clusterUnhealthy:
			log.Println("RECIEVED MSG OF UNHEALTHY CLUSTER.")
		case <-time.After(time.Second*2):
			log.Println("2 secconds passed without unhealthy cluster check")
		}
	}


}
func (k *KubeClient) saveImages(){
	// Save etcd images etc and save localy the filepath to latest images.
	// Send images to poc on request.  
	script := "./kube/scripts/snapshot_etcd.sh"
	cmd, err := exec.Command("/bin/bash", script, "192.168.38.111", ETCD_SNAPSHOT_FILENAME).Output()
	if err != nil{
		log.Panic("ERROR ON CMD", err)
	}
	log.Println("SUCCESS WITH OUTPUT: " ,string(cmd[:]))
}

// Start cluster from etcd image or yaml init if no etcd image exists. 
func (k *KubeClient) startCluster() error{
	_, err :=  os.Open(ETCD_SNAPSHOT_FILENAME)
	if err != nil {
		log.Println("No previously stored image exists")
		return nil
	}
	script := "./kube/scripts/restore_etcd.sh"
	cmd, err := exec.Command("/bin/bash", script, ETCD_SNAPSHOT_FILENAME).Output()
	if err != nil{
		log.Panic("ERROR ON RESTORE ETCD SHELL", err)
	}
	log.Println("SUCCED RESTORE IMG WITH OUTPUT:", string(cmd[:]))

	//k.cluster.AppsV1().RESTClient().Post().Body
	log.Println("GOT CALL TO START CLUSTER.")
	return nil
}

func (k *KubeClient) stopCluster(){
	// Get confirmation from candidate (if possible) before complete stopage.
	// Shutdown cluster. 
}
func (k *KubeClient) startHealthChecks(abort chan bool) chan bool{
	clusterUnhealthy := make(chan bool)
	go func(cu chan bool, abort chan bool){
		for{
			nodeUnhealthy := make(chan bool)
			podUnhealthy := make(chan bool)
			success := make(chan bool)
			go k.checkPodStatus(podUnhealthy)
			go k.checkNodeStatus(nodeUnhealthy)
			go func(){
				statPod := <-podUnhealthy
				statNode := <-nodeUnhealthy
				if statNode && statPod {
					success <- true
					return
				}
				success <- false
			}()
			select{
			case <-abort:
				logCluster("ABORTING HEALTH CHECKS")
				return
			case health:= <-success:
				if health {
					logCluster("HEALTHY CLUSTER")
					time.Sleep(time.Second*TIME_SEC_HEALTH_CHECK)
				}else{
					logCluster("UNHEALTHY CLUSTER.")
					cu <- true
					return
				}
			}
		}
	}(clusterUnhealthy, abort)
	return clusterUnhealthy
}

func logCluster(msg string){
	log.Printf("CLUSTER: %s", msg)	
}





