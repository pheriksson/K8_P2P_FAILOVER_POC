package main

import (
	"fmt"
	"log"
	"time"
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/network"
)

func main(){
	log.Println("Starting app")
	n := network.InitNetwork("localhost",9999);
	go n.Listen()
	test := network.InitNetwork("localhost",9998);
	go test.Listen()
	go func(){
	for{
		msg := "Testing basic network func"
		fmt.Printf("[%s] sending msg: [%s]", n.Addr.String(), msg)
		test.SendRequest(n.Addr, []byte(msg))	
		time.Sleep(5*time.Second)
	}
	}()
	for{
		fmt.Println("sleeping...")	
		time.Sleep(10*time.Second)
	}

}


