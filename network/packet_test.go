package network

import (
	"bytes"
	"net"
	"testing"
)

func TestEncodeDecode(t *testing.T){
	// Init packet attributes.
	wantAddr := &net.UDPAddr{IP:net.ParseIP("127.0.0.1"), Port: 35000}
	wantData := []byte{1,2,3,4,5}

	// Init packet object.
	wantPacket := Packet{wantAddr, wantData} 

	encodePacket, err := Encode(wantPacket)
	if err != nil{
		t.Errorf("Failed encode packet: %s", err)
	}
		
	decodePacket, err := Decode(encodePacket)
	if err != nil{
		t.Errorf("Failed decode packet: %s", err)
	}
	
	if !bytes.Equal(decodePacket.Data, wantData){
		t.Errorf("\nWANTED: %b \nGOT: %b", wantData, decodePacket.Data)
	}
	if wantAddr.IP.String() != decodePacket.Caller.IP.String(){
		t.Errorf("\nWANTED: %s \nGOT: %s", wantAddr.IP.String(), decodePacket.Caller.IP.String())
	}
	if wantAddr.Port != decodePacket.Caller.Port{
		t.Errorf("\nWANTED: %d \nGOT: %d", wantAddr.Port, decodePacket.Caller.Port)
	}
}



