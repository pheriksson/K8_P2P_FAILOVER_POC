package agent

import (
	"net"
	"bytes"
	"testing"
)

func TestEncodeDecode(t *testing.T){
	// Init packet attributes.
	wantAddr := &net.UDPAddr{}
	wantTime := []byte{1}
	wantData := []byte{2}

	// Init packet object.
	wantPacket := Packet{wantAddr, wantTime, wantData} 

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
	
	if !bytes.Equal(decodePacket.Time, wantTime){
		t.Errorf("\nWANTED: %b \nGOT: %b", wantTime, decodePacket.Time)
	}
	
}



