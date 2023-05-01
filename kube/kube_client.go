package kube

import (
	"context"
	"fmt"
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
	TIME_SEC_HEALTH_CHECK = 10
	TIME_SEC_ANNOUNCE_SERVICES = 10
	TIME_SEC_CHECK_CLUSTER_NODES = 20
)

type KubeClient struct{
	objectChannel <-chan KubeCmd 
	cluster *kubernetes.Clientset
	activeNodes []string
	nodeChan chan<-[]string
	activePorts []int
	portChan chan<-[]int
}


func InitKubeClient(confPath string, nodeChan chan<-[]string, portChan chan<-[]int, newObject <-chan KubeCmd) *KubeClient{
	config, err := clientcmd.BuildConfigFromFlags("", confPath)
	if err != nil{
		log.Panic("FAILED TO INIT KUB CLUSTER CLIENT, REASON:", err)
	}
	cluster, err := kubernetes.NewForConfig(config)
	if err != nil{
		log.Panic("FAILED TO INITATE KUB CLUSTER CLIENT. REASON:", err)
	}
	return &KubeClient{
		nodeChan: nodeChan,
		portChan: portChan,
		cluster: cluster,
		objectChannel: newObject,
	};
}


func (k *KubeClient) getClusterNodePorts() ([]int, error){
	services, err := k.cluster.CoreV1().Services("").List(context.TODO(), v1.ListOptions{})
	if err != nil{
		logCluster("FAILED TO GET SERVICES, REASON: "+err.Error())
		return []int{}, err
	}
	svcPorts := make([]int, len(services.Items))
	for _, svc := range services.Items{
		if (svc.Spec.Type == "NodePort"){
			for _, port := range svc.Spec.Ports{
				svcPorts = append(svcPorts, int(port.NodePort))
			}
		}
	}
	return svcPorts, nil 
}


// For proxy, simply fetch addresses and service ports -> return to proxy and loadbalance.
func (k *KubeClient) getClusterNodeAddresses() ([]string, error){
	nodes ,err := k.cluster.CoreV1().Nodes().List(context.TODO(), v1.ListOptions{})
	if err != nil{
		logCluster("FAILED TO GET NODE, REASON: "+err.Error())
		return []string{}, err 
	}
	nodeAddrs := make([]string, len(nodes.Items))
	for _, node := range nodes.Items{
		for _, nodeAddr  := range node.Status.Addresses{
			if nodeAddr.Type == "InternalIP" {
				nodeAddrs = append(nodeAddrs, nodeAddr.Address)
			}
		}
	}
	return nodeAddrs, nil
}

func (k *KubeClient) checkPodStatus(health chan bool){
	// Will query all pods from ALL namespaces. 
	pods, err := k.cluster.CoreV1().Pods("").List(context.TODO(), v1.ListOptions{})
	if err != nil{
		logCluster("FAILED TO GET PODS REASON: "+err.Error())
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
	log.Println("TODO: Start cluster operations.")
	go k.startAnnounceClusterNodes()
	k.announceClusterPorts()

	abort := make(chan bool)
	// TODO, set cluster unhealthy for proxy to simply redirect no matter what. 
	clusterUnhealthy := k.startHealthChecks(abort)
	for{
		select{
		case <-clusterUnhealthy:
			log.Println("<--- UNHEALTHY CLUSTER - FORWARD TO PROXY ONLY --->")
		case update := <-k.objectChannel:
			k.updateCluster(update)
		case <-time.After(time.Second*TIME_SEC_ANNOUNCE_SERVICES):
			k.announceClusterPorts()
		}
	}
}

func (k *KubeClient) startAnnounceClusterNodes(){
	for{
		nodes, err := k.getClusterNodeAddresses()
		if err != nil{
			log.Println("No cluster node accessible.")
			k.nodeChan<-[]string{}
		}else{
			k.nodeChan<-nodes
		}
		time.Sleep(time.Second*TIME_SEC_CHECK_CLUSTER_NODES)
	}
}
func (k *KubeClient) announceClusterPorts() {
	svcPorts, err := k.getClusterNodePorts()
	if err != nil{
		log.Panic("ERROR GETTING NODE PORTS: ",err)
	}
	go func(){k.portChan<-svcPorts}()
} 


func (k *KubeClient) updateCluster(update KubeCmd){
	switch update.Type{
	case CREATE_SERVICE:
		err := CreateNodePort(k.cluster, update.ObjectService)
		if err != nil{
			log.Println("FAILED TO CREATE SERVICE:", err)
			return
		}
		log.Println("CREATED NEW SERVICE AT PORT:", update.ObjectService.Port.NodePort)
	        k.announceClusterPorts()
	case CREATE_DEPLOYMENT:
		err := CreateDeployment(k.cluster, update.ObjectDeployment)
		if err != nil{
			log.Println("FAILED TO CREATE DEPLOYMENT:", err)
			return
		}
		log.Println("CREATED OBJECT DEPLOYMENT: ", update.ObjectDeployment.Name)
	}
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





