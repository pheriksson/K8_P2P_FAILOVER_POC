package main

import (
	"log"
	"os"
	"strconv"

	"github.com/pheriksson/K8_P2P_FAILOVER_POC/poc"
)

func main(){
	log.Println("Starting app")
	argsList := os.Args[1:]
	nodeCase, _ := strconv.Atoi(argsList[0])	

	addrs1 := "127.0.0.1"
	addrs2 := "127.0.0.2"
	addrs3 := "127.0.0.3"
	switch nodeCase{
	case 0:
		x := poc.InitPoC(addrs1, 9999)
		x.TestingRegisterPeer(addrs2, "ADDRS2")	
		x.TestingRegisterPeer(addrs3, "ADDRS3")	
		x.StartPoc()
	case 1:
		x := poc.InitPoC(addrs2, 9999)
		x.TestingRegisterPeer(addrs1, "ADDRS1")	
		x.TestingRegisterPeer(addrs3, "ADDRS3")	
		x.StartPoc()
	case 2:
		x := poc.InitPoC(addrs3, 9999)
		x.TestingRegisterPeer(addrs1, "ADDRS1")	
		x.TestingRegisterPeer(addrs2, "ADDRS2")	
		x.StartPoc()
	default:
		log.Println("FAILED TO START")
	}
}


