package main;  

import (
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/agent"
)

func main(){
	n := agent.InitNetwork("127.0.0.1",8080);
	n.Listen();
}


