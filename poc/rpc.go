package poc

import(
	"encoding/gob"
	"bytes"
	"log"
)

type RequestVote struct{
	term int
	leader string
}

type Vote struct{
	term int
	agreed bool
}

type RequestVolumes struct{
	term int
}

type TestPackage interface {
	Vote | RequestVolumes | RequestVote 
}

type PackageEncoder interface{
	decode() (TestPackage, error)
	encode() ([]byte,error)
}


func decode(rawPacket []byte) (TestPackage, error){
	buffer := bytes.NewBuffer(rawPacket)
	dec := gob.NewDecoder(buffer)
	var packet TestPackage
	err := dec.Decode(&packet);
	if err != nil{
		log.Printf("DECODE ERROR: %s", err)
		return packet, err
	}
	return packet, nil;
}

func (tp TestPackage) encode() ([]byte, error){
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer);
	err := enc.Encode(tp)
	if err != nil{
		log.Printf("ENCODE ERROR: %s",err)
		return nil, nil
	}

	byteBuff := make([]byte, 2048)
	n, _ := buffer.Read(byteBuff);
	return byteBuff[:n], nil
}






