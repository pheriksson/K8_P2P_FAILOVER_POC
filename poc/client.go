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

const (
	PROXY_PORT=9996
	RAFT_PORT=9997
)


type PoC struct{
	kube		*kube.KubeClient	
	proxy           *proxy.Proxy
	cli		string
	cliChan		<-chan	string 
	raft *raft.RAFT
	cliToRaft chan<- kube.KubeCmd
}


func InitPoC(ip string, kubeConf string) *PoC{


	// Raft/Kube synchronization
	clusterNodes := make(chan []string)
	clusterPorts := make(chan []int)


	cliToRaft := make(chan kube.KubeCmd) 
	r, fromRaft := raft.InitRaft(ip, RAFT_PORT, cliToRaft)


	pr := proxy.InitProxy(ip, PROXY_PORT,  clusterNodes, clusterPorts) 

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
				cmd<-kube.KubeCmd{Type: kube.CREATE_SERVICE}
			case 2:
				cmd<-kube.KubeCmd{Type: kube.DELETE_SERVICE}
			case 3:
				cmd<-kube.KubeCmd{Type: kube.CREATE_DEPLOYMENT}
			case 4:
				cmd<-kube.KubeCmd{Type: kube.DELETE_DEPLOYMENT}
			}
		}
	}()
	return cmd
}


