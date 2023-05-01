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
	proxyPeerCh	chan<- []string
	cli		string
	cliChan		<-chan	string 
	raft *raft.RAFT
	toRaft chan<- kube.KubeCmd
}


func InitPoC(ip string, kubeConf string) *PoC{
	toRaft := make(chan kube.KubeCmd) 
	r, fromRaft := raft.InitRaft(ip, RAFT_PORT, toRaft)

	clusterNodes := make(chan []string)
	clusterPorts := make(chan []int)
	peerNodesCh := make(chan []string)

	pr := proxy.InitProxy(ip, PROXY_PORT,  clusterNodes, clusterPorts, peerNodesCh) 

	// Sending raftRead to kubeClient, meaning any confirmed raft read will be executed by kubeclient.
	k := kube.InitKubeClient(kubeConf, clusterNodes, clusterPorts, fromRaft) 

	p := PoC{
		raft: r,
		toRaft: toRaft,
		kube: k,
		proxy: pr,
		proxyPeerCh: peerNodesCh,
	}
	return &p
}


func (p *PoC) StartPoc(){
	go p.raft.StartRaft()
	cmdCh := p.startCli()
	t := 0
	for {
		select{
		case <-time.After(time.Second):
			t+=1	
		case cmd := <- cmdCh:
			p.toRaft<-cmd
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


func (p *PoC) TestingRegisterPeer(ip string, hostname string){
	p.raft.TestRegisterPeer(ip, hostname)
}

