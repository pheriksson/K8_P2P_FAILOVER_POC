package agent;

import (
	"log"
	"net"
)

type Network struct {
	addr			*net.UDPAddr
	placeHolderChannel	chan string
}

func InitNetwork(ip string, port int) *Network{
	addr := &net.UDPAddr{IP:net.ParseIP(ip), Port: port};
	return &Network{addr, make(chan string)}
}

func (n *Network) Listen(){
	conn, err := net.ListenUDP("udp", n.addr);
	if err != nil {
		log.Panic("CANNOT BIND TO ADDR:", n.addr);
	}
	defer conn.Close();
	for {
		packetBuffer := make([]byte, 1024);
		l, caller, err := conn.ReadFromUDP(packetBuffer)
		if err != nil{
			log.Printf("FAILED TO READ FROM: [%s]", caller);
		}else{
			go n.handleRequest(packetBuffer[:l]);
		}
	}
}

func (n *Network) handleRequest(buffer []byte){
	log.Printf("RECIEVED: %s", string(buffer));
}
