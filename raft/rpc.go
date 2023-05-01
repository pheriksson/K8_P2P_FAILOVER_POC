package raft

import(
	"encoding/gob"
	"bytes"
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/kube"
	"log"
)

type RequestVote struct{
	Term int
	CandidateAddr string 
	LogIndex int
	Agreed bool
}

type AppendEntry struct {
	Term int
	LeaderAddr string
	LogIndex int 
	NewObject kube.KubeCmd 
	Confirmed bool
}

type FetchMissingEntries struct{
	Term int
	LeaderAddr string
	LogIndex int
	NewObject kube.KubeCmd 
}

type AppendEntryResponse struct{
	Term int
	LeaderAddr string
	LogIndex int
	Confirmed bool 
}

type Payload interface {
	 RequestVote | AppendEntry | AppendEntryResponse | FetchMissingEntries
}

const (
	MAX_RPC_SIZE = 2048
)

func decodePayload[T Payload](rawPacket []byte) (T, error){
	buffer := bytes.NewBuffer(rawPacket)
	dec := gob.NewDecoder(buffer)
	var packet T
	err := dec.Decode(&packet);
	if err != nil{
		log.Printf("DECODE ERROR: %s", err)
		return packet, err
	}
	return packet, nil;
}


func encodePayload[T  Payload](structInstance T) ([]byte, error){
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer);
	err := enc.Encode(structInstance)
	if err != nil{
		log.Printf("ENCODE ERROR: %s",err)
		return nil, nil
	}
	byteBuff := make([]byte, MAX_RPC_SIZE)
	n, _ := buffer.Read(byteBuff);
	return byteBuff[:n], nil
}






