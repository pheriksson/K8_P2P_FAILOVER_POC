package main

import (
	"log"
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/poc"
)

func main(){
	log.Println("Starting app")
	x := poc.InitPoC("127.0.0.1", 9999)
	x.StartPoc()

}


