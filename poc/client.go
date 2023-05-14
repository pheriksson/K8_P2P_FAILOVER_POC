package poc

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/pheriksson/K8_P2P_FAILOVER_POC/kube"
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/proxy"
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/raft"
)



type PoC struct{
	kube		*kube.KubeClient	
	proxy           *proxy.Proxy
	cli		string
	cliChan		<-chan	string 
	raft		*raft.RAFT
	cliToRaft	chan<- kube.KubeCmd
}


func InitPoC(ip string, kubeConf string, raftPort int, proxyPort int) *PoC{


	// Raft/Kube synchronization
	clusterNodes := make(chan []string)
	clusterPorts := make(chan []int)


	cliToRaft := make(chan kube.KubeCmd) 
	r, fromRaft := raft.InitRaft(ip, raftPort, cliToRaft)


	pr := proxy.InitProxy(ip, proxyPort,  clusterNodes, clusterPorts) 

	// Sending raftRead to kubeClient, meaning any confirmed raft read will be executed by kubeclient.
	k := kube.InitKubeClient(kubeConf, clusterNodes, clusterPorts, fromRaft) 

	p := PoC{
		raft: r,
		cliToRaft: cliToRaft,
		kube: k,
		proxy: pr,
	}
	return &p
}


func (p *PoC) StartPoc(peers []string){
	// Peers list.
	go p.raft.Start(peers)
	go p.proxy.Start(peers)
	go p.kube.Start()
	cmdCh := p.startCli()
	t := 0
	for {
		select{
		case <-time.After(time.Second):
			t+=1	
		case cmd := <- cmdCh:
			p.cliToRaft<-cmd
		}
	}
}

func (p *PoC) startCli() (chan kube.KubeCmd){
	log.Println("Starting limited cli")
	cmd := make(chan kube.KubeCmd)
	go func(){
		var cmdString string
		for{
			_, err := fmt.Scanln(&cmdString)
			if err != nil{
				log.Println("ERROR PARSING INPUT")
				continue
			}
			num, err := strconv.Atoi(cmdString)
			if err != nil{
				log.Println("ERROR PARSING INPUT: ", cmdString)
				continue
			}
			switch num{
			case 1:
				msg := TestGenDeploymentCmd()
				cmd<-msg
			case 2:
				msg := TestGenNodeportCmd()
				cmd<-msg
			}
		}
	}()
	return cmd
}

func TestGenDeploymentCmd() kube.KubeCmd{
	err, obj := kube.CreateDeploymentObject(
		"poc-deployment",
		5,
		"app",
		"cluster-loc",
		"app",
		"cluster-loc",
		"server-cluster-status",
		"seaweed39kelp/poctesting:1.0",
		80,
		"http",
	)
	if err != nil{
		log.Panic("Failed to create object.")
	}
	return kube.KubeCmd{Type: kube.CREATE_DEPLOYMENT, ObjectDeployment: *obj}
}
func TestGenNodeportCmd() kube.KubeCmd{
	err, svc := kube.CreateNodePortObject(
		"poc-node-port",
		"",
		80,
		80,
		30010,
		"app",
		"cluster-loc",
	)
	if err != nil{
		log.Panic("Failed to create object.")
	}
	return kube.KubeCmd{Type: kube.CREATE_SERVICE, ObjectService: *svc}
}

