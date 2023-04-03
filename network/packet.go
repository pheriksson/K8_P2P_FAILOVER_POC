package network

import (
	"bytes"
	"encoding/gob"
	"log"
	"net"
)

type Packet struct {
	Caller *net.UDPAddr
	Data   []byte
	Type DataType
}

type DataType int

const(
	RAFT_VOTE DataType = iota
	RAFT_REQUEST_VOTE
	REQUEST_PERSISTENT_VOLUMES
)

func NewPacket(c *net.UDPAddr, data []byte) *Packet{
	p := Packet{Caller: c, Data: data}
	return &p
}

func Encode(p Packet) ([]byte, error){
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer);
	err := enc.Encode(p)
	if err != nil{
		log.Printf("ENCODE ERROR: %s",err)
		return nil, nil
	}

	byteBuff := make([]byte, 2048)
	n, _ := buffer.Read(byteBuff);
	return byteBuff[:n], nil
}

func Decode(rawPacket []byte) (Packet, error){
	buffer := bytes.NewBuffer(rawPacket)
	dec := gob.NewDecoder(buffer)
	var packet Packet
	err := dec.Decode(&packet);
	if err != nil{
		log.Printf("DECODE ERROR: %s", err)
		return packet, err
	}
	return packet, nil;
}

