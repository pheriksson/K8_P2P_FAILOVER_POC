package network;

import (
	"log"
	"fmt"
	"net"
)

type Network struct {
	Addr		*net.UDPAddr
	pocChannel	chan<- Packet 
}




func InitNetwork(ip string, port int) *Network{
	addr := &net.UDPAddr{IP:net.ParseIP(ip), Port: port};
	return &Network{addr, make(chan Packet)}
}

func (n *Network) Listen(){
	fmt.Println("listening on port...")
	conn, err := net.ListenUDP("udp", n.Addr);
	if err != nil {
		log.Panic("CANNOT BIND TO ADDR:", n.Addr);
	}
	defer conn.Close();
	for {
		packetBuffer := make([]byte, 1024);
		l, caller, err := conn.ReadFromUDP(packetBuffer)
		if err != nil{
			log.Printf("FAILED TO READ FROM: [%s]", caller);
		}else{
			go n.handleRequest(caller, packetBuffer[:l]);
		}
	}
}

func (n *Network) handleRequest(caller *net.UDPAddr, packet []byte){
	decPacket, err := Decode(packet)
	log.Printf("RECIEVED MSG: [%s]", decPacket.Data)
	if err != nil {
		log.Printf("FAILED TO DECODE INCOMING PACKET FROM %s. ERROR: %s", caller.String(), err)
	}
	go func(){
		//log.Printf("TESTING ECHO TO: [%s:%d]", decPacket.Caller.IP, decPacket.Caller.Port)
		//n.SendRequest(decPacket.Caller, []byte{1})
		n.pocChannel <- decPacket	
	}()
}

func (n *Network) SendRequest(target *net.UDPAddr, data []byte){
	packet := NewPacket(n.Addr, data)
	encPacket, err := Encode(*packet)
	if err != nil {
		log.Printf("FAILED TO ENCODE PACKET ADDRESSED TO %s. ERROR: %s", target,err)
	}
	targetAddr, err := net.ResolveUDPAddr("udp", target.String())
	if err != nil {
		log.Printf("FAILED TO RESOLVE UDP ADDR ADDRESSED TO %s. ERROR: %s", target, err)
	}
	conn, err := net.DialUDP("udp", nil, targetAddr)
	if err != nil {
		log.Printf("FAILED TO CONNECT TO UDP ENDPOINT %s. ERROR: %s", targetAddr.IP, err)
	}
	defer conn.Close()
	_, err = conn.Write(encPacket)	
	if err != nil{
		log.Printf("FAILED TO WRITE TO %s. ERROR: %s", conn.LocalAddr(), err)
	}
}






