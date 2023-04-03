package poc

import(
	"encoding/gob"
	"bytes"
	"log"
)


type RequestVote struct{
	Term int
	Leader string
}

type Vote struct{
	Term int
	Agreed bool
}

type RequestVolumes struct{
	Term int
}

type Payload interface {
	Vote | RequestVolumes | RequestVote
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






