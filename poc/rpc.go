package poc

import(
	"encoding/gob"
	"bytes"
	"log"
)

// TODO: Data transfer from candidate to members.
// -> When candidate recieves volume, simply send to rest of members. 
type VoteLeader struct{
	Term int
	LeaderAddr string
	Agreed bool
}
// Used as candidate confirmation of leader and candidate 
type VoteCandidate struct{
	Term int
	LeaderAddr string
	CandidateAddr string
	Agreed bool	
}
// Used as ping by leader to all - discovery of member to candidate.
// And to be used by candidate to notify of leader health deteriating. 
type Health struct{
	Term int
	LeaderAddr string
	CandidateAddr string
	Stable	bool
}

type RequestVolumes struct{
	Term int
}

type Payload interface {
	VoteLeader | VoteCandidate | Health | RequestVolumes
}

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
	byteBuff := make([]byte, 2048)
	n, _ := buffer.Read(byteBuff);
	return byteBuff[:n], nil
}






