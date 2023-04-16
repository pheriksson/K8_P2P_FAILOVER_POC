package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/pheriksson/K8_P2P_FAILOVER_POC/poc"
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/proxy"
)

func main(){
	log.Println("Starting app")
	argsList := os.Args[1:]
	nodeCase, _ := strconv.Atoi(argsList[0])	
	
	addrs1 := "127.0.0.1"
	p1 := make(chan []int)
	l1 := make(chan string)
	addrs2 := "127.0.0.2"
	p2 := make(chan []int)
	l2 := make(chan string)
	addrs3 := "127.0.0.3"
	p3 := make(chan []int)
	l3 := make(chan string)
	x := proxy.InitProxy(l1, p1, addrs1)
	t2 := proxy.InitProxy(l2, p2, addrs2)
	t3 := proxy.InitProxy(l3, p3, addrs3)
	go func(){
		// Init t1
		go t2.Start()
		for{
			p2<-[]int{9997}
			l2<-addrs2
			time.Sleep(time.Second*10000)
		}
	}()
	go func(){
		// Init t1
		go t3.Start()
		for{
			p3<-[]int{9997}
			l3<-addrs2
			time.Sleep(time.Second*1000)
		}
	}()
	go func(){
		for{
		p1<-[]int{9997}
		l1<-addrs2
		time.Sleep(time.Second*1000)
		}
	}()
	x.Start()
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


