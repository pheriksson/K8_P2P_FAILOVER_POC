package proxy

import(
	"log"
	"net"
	"encoding/gob"
	"bytes"
)

const(
	PROXY_MAX_PACKET_SIZE=1024
)

type ProxyPacket struct{
	Failed bool
	Port int
	Data []byte
}


func ReadProxyPacket(s net.Conn) (ProxyPacket, error){
	buffer := make([]byte, PROXY_MAX_PACKET_SIZE)
	_, err := s.Read(buffer)
	if err != nil{
		return ProxyPacket{}, err
	}
	return ProxyPacket{}, nil
}

func WriteProxyPacket(s net.Conn, pp ProxyPacket) error{
	data, err := pp.encode()
	if err != nil{
		return err
	}
	_, err = s.Write(data)
	if err != nil{
		return err
	}
	return nil
} 

func (p *ProxyPacket) encode() ([]byte, error){
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

func decode(data []byte) (ProxyPacket, error){
	buffer := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buffer)
	var packet ProxyPacket
	err := dec.Decode(&packet);
	if err != nil{
		log.Printf("DECODE ERROR: %s", err)
		return packet, err
	}
	return packet, nil;
}
