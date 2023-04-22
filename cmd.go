package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pheriksson/K8_P2P_FAILOVER_POC/poc"
	"k8s.io/client-go/util/homedir"
)

func main(){
	var clusterConfPath string
	if home:= homedir.HomeDir(); home != ""{
		flag.StringVar(&clusterConfPath, "configPath",filepath.Join(home,".kube","config"), "Path to kubeconfig file")
	}else{
		flag.StringVar(&clusterConfPath, "configPath","","Path to kubeconfig file")
	}
	flag.Parse()
	clusterConfPath = "/home/superhome/.kube/config"



	argsList := os.Args[1:]
	nodeCase, _ := strconv.Atoi(argsList[0])	
	
	addrs1 := "127.0.0.1"
	addrs2 := "127.0.0.2"
	addrs3 := "127.0.0.3"
	switch nodeCase{
	case 0:
	        log.Println("Starting app as ", addrs1)
		x := poc.InitPoC(addrs1, 9999, clusterConfPath)
		x.TestingRegisterPeer(addrs2, "ADDRS2")	
		x.TestingRegisterPeer(addrs3, "ADDRS3")	
		x.StartPoc()
	case 1:
	        log.Println("Starting app as ", addrs2)
		x := poc.InitPoC(addrs2, 9999, clusterConfPath)
		x.TestingRegisterPeer(addrs1, "ADDRS1")	
		x.TestingRegisterPeer(addrs3, "ADDRS3")	
		x.StartPoc()
	case 2:
	        log.Println("Starting app as ", addrs3)
		x := poc.InitPoC(addrs3, 9999, clusterConfPath)
		x.TestingRegisterPeer(addrs1, "ADDRS1")	
		x.TestingRegisterPeer(addrs2, "ADDRS2")	
		x.StartPoc()
	default:
		log.Println("FAILED TO START")
	}
}


