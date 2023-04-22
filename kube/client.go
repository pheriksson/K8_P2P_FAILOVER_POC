package kube

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
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
	ETCD_SNAPSHOT_FILENAME = "etcd_snapshot.db"
)

type KubeClient struct{
	activeCluster bool 
	fromPoc <-chan KubeMsg
	toPoc chan<- KubeMsg 
	cluster *kubernetes.Clientset
	activeNodes []string
	announceNodes chan<-[]string
	activePorts []int
	announcePorts chan<-[]int
}

type KubeMsg int

const(
	START_CLUSTER KubeMsg = iota
	STOP_CLUSTER
	UNHEALTHY_CLUSTER
	CONFIRM_EARLY_FAILOVER
	PORT_ANNOUNCEMENT
)

// Need to address this stuff aswell
type NodeAddress string
type ServicePorts int

func InitKubeClient(confPath string, nodeIpChan chan<-[]string, portChan chan<-[]int, fromPoc <-chan KubeMsg, toPoc chan<- KubeMsg) *KubeClient{
	config, err := clientcmd.BuildConfigFromFlags("", confPath)
	if err != nil{
		log.Panic("FAILED TO INIT KUB CLUSTER CLIENT, REASON:", err)
	}
	cluster, err := kubernetes.NewForConfig(config)
	if err != nil{
		log.Panic("FAILED TO INITATE KUB CLUSTER CLIENT. REASON:", err)
	}
	return &KubeClient{
		activeCluster: false,
		announceNodes: nodeIpChan,
		announcePorts: portChan,
		cluster: cluster,
		fromPoc: fromPoc,
		toPoc: toPoc,
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
	os.Exit(0)
	// Stop conditions -> health checks start to fail -> init early failover and wait.
	// Or abort from poc signaling new leader enroled  (self is not leader). 
	// TODO: Send active nodes to proxy. Can be done by all the proxies to prepare.
	k.saveImages()
	func(){
	time.Sleep(time.Second*3)
	err := k.startCluster()
	if err != nil{
		log.Println("FAILED TO START CLUSTER:",err)
	}
	}()
	// Init proxy ports/nodes after succesfull restoration.
	nodes, err := k.getClusterNodeAddresses()
	if err != nil{
		log.Panic("ERROR GETTING NODES: ",err)
	}
	go func(){k.announceNodes <- nodes}()

	svcPorts, err := k.getClusterNodePorts()
	if err != nil{
		log.Panic("ERROR GETTING NODE PORTS: ",err)
	}
	go func(){k.announcePorts<-svcPorts}()
	if err != nil{
		log.Panic("CANNOT START CLUSTER, REASON: ",err)
	}
	abort := make(chan bool)
	clusterUnhealthy := k.startHealthChecks(abort)
	for{
		select{
		case <-clusterUnhealthy:
			log.Println("RECIEVED MSG OF UNHEALTHY CLUSTER.")
			k.toPoc<- UNHEALTHY_CLUSTER
		// Need to organice communication from and to PoC.
		case cmd := <-k.fromPoc:
			log.Println("RECIEVED MSG FOR POC ", cmd)
			k.handleMsg(cmd)
		case <-time.After(time.Second):
			log.Println("")
		case <-time.After(time.Second*TIME_SEC_ANNOUNCE_SERVICES):
			svcPorts, err := k.getClusterNodePorts()
			if err != nil{
				log.Panic("ERROR GETTING NODE PORTS: ",err)
			}
			go func(){
				log.Println("ANNOUNCE CURRENT SERVICES TO PROXY SENDING TO PROXY:", svcPorts)
				k.announcePorts<-svcPorts
			}()
			
		}
	}
}

func (k *KubeClient) handleMsg(cmd KubeMsg){
	switch cmd{
	case START_CLUSTER:
		log.Println("TODO: CALL TO START CLUSTER AND RETURN OK WHEN START CONFIRMED.")
	case STOP_CLUSTER:
		log.Println("TODO: INSERT CALL TO STOP CLUSTER.")
	default:
		log.Println("RECIEVED DEFAULT TAG FOR HANDLEM;SG:", cmd)
         }
}


func (k *KubeClient) saveImages(){
	script := "./kube/scripts/snapshot_etcd.sh"
	_, err := exec.Command("/bin/bash", script).Output()
	if err != nil{
		log.Panic("COULD NOT SAVE ETCD VOLUME, REASON:", err)
	}
}


func (k *KubeClient) startCluster() error{
	defaultSnapshotPath :="/tmp/poc/ETCD_VOLUME_BACKUP"
	_, err :=  os.Open(defaultSnapshotPath)
	if err != nil {
		log.Println("NO PREVIOUSLY STORED IMAGE")
		return nil
	}

	script := "./kube/scripts/restore_etcd.sh"
	_, err = exec.Command("/bin/bash", script).Output()
	if err != nil{
		log.Panic("ERROR ON RESTORE ETCD SHELL", err)
	}
	log.Println("GOT CALL TO START CLUSTER.")
	k.activeCluster = true
	return nil
}


func (k *KubeClient) stopCluster(){
	k.activeCluster = false
	// Delete services.
	// Delete deployments.
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





