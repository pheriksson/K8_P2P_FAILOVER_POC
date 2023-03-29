package network

import (
	"bytes"
	"encoding/gob"
	"log"
	"net"
	"time"
)

const PACKET_TIMESTAMP_FORMAT = "2006-01-02 15:04:05" 

type Packet struct {
	Caller *net.UDPAddr
	TimeStamp string 
	Data   []byte
}

func NewPacket(c *net.UDPAddr, data []byte) *Packet{
	p := Packet{Caller: c, TimeStamp: time.Now().Format(PACKET_TIMESTAMP_FORMAT), Data: data}
	return &p
}



func GetTimeStamp(p *Packet) (time.Time, error){
	t, err := time.Parse(PACKET_TIMESTAMP_FORMAT, (*p).TimeStamp)
	if err != nil{
		log.Printf("TIME PARSE ERROR: %s", err);
		return t, err;
	}
	return t,nil;
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


