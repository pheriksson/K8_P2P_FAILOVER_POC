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
	RAFT_VOTE_LEADER DataType = iota
	RAFT_VOTE_CANDIDATE
	RAFT_HEALTH	
	RAFT_MEMBER_REQUEST_VOTE
	RAFT_CANDIDATE_REQUEST_VOTE
	REQUEST_PERSISTENT_VOLUMES
)

func (d DataType) ToString() string{
	switch (d){
	case RAFT_VOTE_LEADER:
		return "RAFT_VOTE_LEADER"
	case RAFT_VOTE_CANDIDATE:
		return "RAFT_VOTE_CANDIDATE"
	case RAFT_HEALTH:
		return "RAFT_HEALTH"
	case RAFT_MEMBER_REQUEST_VOTE:
		return "RAFT_MEMBER_REQUEST_VOTE"
	default:
		return "UNKNOWN"
	}


}
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

