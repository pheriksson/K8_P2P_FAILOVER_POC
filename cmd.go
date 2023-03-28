package main;  

import (
	"log"
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/agent"
)

func main(){
	log.Println("Starting app")
	n := agent.InitNetwork("0.0.0.0",9999);
	n.Listen();
}


