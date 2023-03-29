package network

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestPacketTimestampFormat(t *testing.T){
	testPacket := NewPacket([]byte{1})
	timeStamp, err := GetTimeStamp(testPacket)
	if err != nil{
		t.Errorf("Could not get valid format for packet timestamp: %s", err)
	}
	if (testPacket.TimeStamp != timeStamp.Format(PACKET_TIMESTAMP_FORMAT)){
		t.Errorf("Invalid time formatting for packets")
	}
}


func TestEncodeDecode(t *testing.T){
	// Init packet attributes.
	wantAddr := &net.UDPAddr{}
	wantTime := time.Now().String() 
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
	if !(decodePacket.TimeStamp == wantTime){
		t.Errorf("\nWANTED: %s \nGOT: %s", wantTime, decodePacket.TimeStamp)
	}	
}



