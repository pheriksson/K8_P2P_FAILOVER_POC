package network

import (
	"fmt"
	"log"
	"net"
	"time"
)
const(
  TIMEOUT_SEC_RECIEVE = 2
  TIMEOUT_SEC_SEND = TIMEOUT_SEC_RECIEVE + 2

)
type Network struct {
	addr		*net.TCPAddr
	pocChan		chan<- RPC
}

type RPC struct{
	P	Packet
	Done	chan []byte
}

func (rpc *RPC) GetPacket() *Packet{
	return &rpc.P
}

func InitNetwork(ip string, portRPC int, ch chan<-RPC) *Network{
	addrRPC := &net.TCPAddr{IP:net.ParseIP(ip), Port: portRPC};
	return &Network{addrRPC, ch}
}

func (n *Network) Listen(){
	ss, err := net.ListenTCP("tcp", n.addr)
	if err != nil{
		log.Panic("CANNOT BIND TO ADDR:", n.addr)
	}
	defer ss.Close()

	log.Println("STARTING MOD RAFT ON:",n.addr)
	for {
		s, err := ss.Accept()
		if err != nil{
			log.Printf("FAILED TO READ SOCKET %s", err)
		}
		go n.handleRequest(s)
	}
}

func (n *Network) handleRequest(s net.Conn){
	defer s.Close()
	buffer := make([]byte, 2048)
        s.Read(buffer)
	decPacket, err := Decode(buffer)
	if err != nil{
		log.Println("FAILED TO READ FROM SOCKET")
	}
	request := RPC{decPacket, make(chan []byte)}
	n.pocChan <- request 
	select{
	case <-time.After(time.Second*TIMEOUT_SEC_RECIEVE):
		return
	case resp := <-request.Done:
		_, err := s.Write(resp)
		if err != nil{
			log.Println("ERROR ON WRITE TO SOCKET:", err)
		}
	}
}

func (n *Network) SendRequest(target *net.TCPAddr, data []byte, dt DataType) error{ 
	packet := NewPacket(n.addr, data, dt)
	encPacket, err := Encode(*packet)
	if err != nil {
		return fmt.Errorf("FAILED TO ENCODE PACKET ADDRESSED TO %s. ERROR: %s",target, err)                
	}
	targetAddr, err := net.ResolveTCPAddr("tcp", target.String())
	if err != nil {
		return fmt.Errorf("FAILED TO RESOLVE TCP ADDR ADDRESSED TO %s. ERROR: %s",target, err)
	}
	s, err := net.DialTCP("tcp", nil, targetAddr)
	if err != nil {
		return fmt.Errorf("FAILED TO CONNECT TO TCP ENDPOINT %s. ERROR: %s",target.IP, err)                
	}
	_, err = s.Write(encPacket)	
	if err != nil{
		return fmt.Errorf("FAILED TO WRITE TO %s. ERROR: %s",s.LocalAddr(), err)
	}
	return nil 
}

func (n *Network) SendRequestAwaitResponse(target *net.TCPAddr, data []byte, dt DataType) (error, chan Packet){ 
	packet := NewPacket(n.addr, data, dt)
	encPacket, err := Encode(*packet)
	if err != nil {
		return fmt.Errorf("FAILED TO ENCODE PACKET ADDRESSED TO %s. ERROR: %s",target, err), make(chan Packet)
	}
	targetAddr, err := net.ResolveTCPAddr("tcp", target.String())
	if err != nil {
		return fmt.Errorf("FAILED TO RESOLVE TCP ADDR ADDRESSED TO %s. ERROR: %s",target, err), make(chan Packet)
	}
	s, err := net.DialTCP("tcp", nil, targetAddr)
	if err != nil {
		return fmt.Errorf("FAILED TO CONNECT TO TCP ENDPOINT %s. ERROR: %s",target.IP, err), make(chan Packet)
	}
	_, err = s.Write(encPacket)	
	if err != nil{
		return fmt.Errorf("FAILED TO WRITE TO %s. ERROR: %s",s.LocalAddr(), err), make(chan Packet)
	}
	resp := make(chan Packet)
	go func(){
		defer s.Close()
		respBuffer := make([]byte, 2048)
		s.Read(respBuffer)
		decPacket, err := Decode(respBuffer)
		if err != nil{
			return
		}
		go func(){
			resp <- decPacket
		}()
	}()
	return nil, resp 
}

