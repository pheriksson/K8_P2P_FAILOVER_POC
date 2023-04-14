package kube

import (
	"context"
	"log"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
)

type KubeClient struct{
	clusterInput chan string
	pocCommands chan string
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

func (k *KubeClient) checkPodStatus(health chan bool){
	// Will query all pods from ALL namespaces. 
	pods, err := k.cluster.CoreV1().Pods("").List(context.TODO(), v1.ListOptions{})
	if err != nil{
		log.Println("FAILED TO GET PODS, REASON: ",err)
	}
	confirmedHealthy := 0
	for _, pod := range pods.Items{
		if (pod.Status.Phase == POD_STATUS_HEALTHY){
			confirmedHealthy++
		}
	}
	log.Printf("Healthy pods: %d/%d",confirmedHealthy, len(pods.Items))
	if confirmedHealthy == len(pods.Items){health<- true}else{health<- false}
}


func (k *KubeClient) checkNodeStatus(health chan  bool){
	nodes ,err := k.cluster.CoreV1().Nodes().List(context.TODO(), v1.ListOptions{})
	if err != nil{
		log.Println("FAILED TO GET NODES, REASON: ",err)
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
	log.Printf("Healthy nodes: %d/%d", confirmedHealthy, len(nodes.Items))
	if confirmedHealthy == len(nodes.Items){health<- true} else {health<- false}
}
func (k *KubeClient) Start(){
	// If etcd instance exists start from that, else start from init yaml.
	log.Println("TODO: Start cluster operations.")
	// Have channel to indicate failover. 
	// Stop conditions -> health checks start to fail -> init early failover and wait.
	// Or abort from poc signaling new leader enroled  (self is not leader). 
	err := k.startCluster()
	if err != nil{
		log.Panic("CANNOT START CLUSTER, REASON: ",err)
	}
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
}
func (k *KubeClient) startCluster() error{
	// 
	log.Println("GOT CALL TO START CLUSTER.")
	return nil
}
func (k *KubeClient) stopCluster(){
	// Get confirmation from candidate (if possible) before complete stopage.
	// Shutdown cluster. 
}
func (k *KubeClient) startHealthChecks(abort chan bool) chan bool{
	// Simply  check nodes/pods for health status.
	// If below threshold -> signal to poc to failover.

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
				log.Println("ABORTING HEALTH CHECKS")
				return
			case health:= <-success:
				if health {
					log.Println("HEALTHY CLUSTER.")
					log.Println("Entering sleep timeout.")
					time.Sleep(time.Second*5)
				}else{
					log.Println("UNHEALTHY CLUSTER.")
					cu <- true
					return
				}
			}
		}
	}(clusterUnhealthy, abort)
	return clusterUnhealthy
}






