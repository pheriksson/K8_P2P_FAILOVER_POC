package network

import (
	"bytes"
	"encoding/gob"
	"log"
	"net"
)


type Packet struct {
	Caller *net.TCPAddr
	Data   []byte
	Type DataType
}

const(
	PACKET_SIZE = 2048
)

type DataType int

const(
	REQUEST_VOTE DataType = iota
	APPEND_ENTRY
	FETCH_MISSING_ENTRIES
)

// TODO: Error if packet somehow exceds limited packet size.
func NewPacket(c *net.TCPAddr, data []byte, dt DataType) *Packet{
	p := Packet{Caller: c, Data: data, Type: dt}
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
	byteBuff := make([]byte, PACKET_SIZE)
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

