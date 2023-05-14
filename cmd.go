package main

import (
	"flag"
	"log"
	"path/filepath"
	"strings"

	"k8s.io/client-go/util/homedir"

	"github.com/pheriksson/K8_P2P_FAILOVER_POC/poc"
)

func main(){
	var clusterConfPath string
	if home:= homedir.HomeDir(); home != ""{
		flag.StringVar(&clusterConfPath, "configPath",filepath.Join(home,".kube","config"), "Path to kubeconfig file")
	}else{
		flag.StringVar(&clusterConfPath, "configPath","","Path to kubeconfig file")
	}
	peerIps := flag.String("peerips", "", "peer ips, seperated by comma.\nex: -peerips=127.0.0.1,127.0.0.2 ")
	exposeIp := flag.String("ip", "", "expose poc on ip")

	raftPort := flag.Int("raft", 9996, "raft port")
	proxyPort := flag.Int("proxy", 9997, "raft port")

	flag.Parse()


	if len(*exposeIp) == 0 {
		log.Panic("CANNOT START WITHOUT EXPOSING POC ON IP")
	}

	peerIpList := strings.Split(*peerIps, ",")
	if len(peerIpList) == 0 {
		log.Panic("CANNOT START WITHOUT ANY PEERS")
	}
	poc.InitPoC(*exposeIp, clusterConfPath, *raftPort, *proxyPort).StartPoc(peerIpList)
}


