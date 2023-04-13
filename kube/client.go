package kube

import (
	"context"
	"log"

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

func (k *KubeClient) CheckPodStatus(){
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
}


func (k *KubeClient) CheckNodeStatus(){
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
}
func (k *KubeClient) Start(){
	// If etcd instance exists start from that, else start from init yaml.
}
func (k *KubeClient) saveImages(){
	// Save etcd images etc and save localy the filepath to latest images.
	// Send images to poc on request.  
}
func (k *KubeClient) stopCluster(){
	// Get confirmation from candidate (if possible) before complete stopage.
	// Shutdown cluster. 
}
func (k *KubeClient) startHealthChecks() chan bool{
	// Simply  check nodes/pods for health status.
	// If below threshold -> signal to poc to failover.
	abort := make(chan bool)
	go func(){
	}()
	return abort
}






