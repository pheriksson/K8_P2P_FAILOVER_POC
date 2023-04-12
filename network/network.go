package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
)
const(
  TIMEOUT_SEC_SEND = TIMEOUT_SEC_RECIEVE + 2
  TIMEOUT_SEC_RECIEVE = 2

)
type Network struct {
	AddrRPC		*net.TCPAddr
	AddrFileTransfer *net.TCPAddr
	pocChan chan<- RPC
}

type RPC struct{
	P	Packet
	Done	chan []byte
}

func (rpc *RPC) GetPacket() *Packet{
	return &rpc.P
}

func InitNetwork(ip string, portRPC int, portFileTransfer int, ch chan<-RPC) *Network{
	addrRPC := &net.TCPAddr{IP:net.ParseIP(ip), Port: portRPC};
	addrFileStream := &net.TCPAddr{IP:net.ParseIP(ip), Port: portFileTransfer}
	return &Network{addrRPC, addrFileStream, ch}
}

func (n *Network) Listen(){
	ss, err := net.ListenTCP("tcp", n.AddrRPC)
	if err != nil{
		log.Panic("CANNOT BIND TO ADDR:", n.AddrRPC)
	}
	defer ss.Close()
	ssF, err := net.ListenTCP("tcp", n.AddrFileTransfer)

	go func(){
		for{
		sf, err := ssF.Accept()
		if err != nil{
			log.Printf("FAILED TO READ SOCKET %s", err)
		}
		go n.handleHugeFiles(sf)

		}
	}()

	log.Println("STARTING MOD RAFT ON:",n.AddrRPC)
	for {
		s, err := ss.Accept()
		if err != nil{
			log.Printf("FAILED TO READ SOCKET %s", err)
		}
		go n.handleRequest(s)
	}
}

func (n *Network) handleHugeFiles(s net.Conn) (){
	var fileSize int64
	err := binary.Read(s, binary.LittleEndian, &fileSize)	
	if err != nil{
		log.Println("FAILED TO WRITE FILE SIZE", err)
		return
	}
	store, err := os.Create(time.Stamp+"recieved_file.txt")
	if err != nil{
		log.Println("FAILED TO WRITE TO FILE.", err)
		return 
	}
	_, err = io.CopyN(store, s, fileSize)
	if err != nil{
		log.Println("FAILED TO COPY BUFFER TO FILE", err)
	}
}

func (n *Network) SendLargeFile(target *net.TCPAddr,filePath string) chan bool{
	status := make(chan bool)
	go func(){
	byteStream, err := os.ReadFile(filePath)
	if err != nil{
		status <- false
	}
	size := int64(len(byteStream))
	conn, err := net.DialTCP("tcp", nil, target)
	if err != nil{
		status <- false
	}
	err = binary.Write(conn, binary.LittleEndian, size)
	if err != nil{
		status <- false
	}
	_, err = io.CopyN(conn, bytes.NewReader(byteStream), size)
	if err != nil{
		status <- false
	}
	status <- true

	}()
	return status
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
	packet := NewPacket(n.AddrRPC, data, dt)
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

// Func send request await response. 
func (n *Network) SendRequestAwaitResponse(target *net.TCPAddr, data []byte, dt DataType) (error, chan Packet){ 
	packet := NewPacket(n.AddrRPC, data, dt)
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
			// new go func to close socket and only leave channel.
			resp <- decPacket
		}()
	}()
	return nil, resp 
}

