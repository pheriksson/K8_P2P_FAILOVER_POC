package poc

import (
	"fmt"
	"time"
)

const (
	TIME_FAIL_RANGE_NEW_HEARTBEAT_SECCONDS=time.Duration(1*time.Second)
)

type PeerRouting struct{
	peers	map[string]*peer 
}

type peer struct{
	ttl time.Time // Basically if a communication has not been reached before ttl, deem peer as inactive.
	auth string // not used. 
	contact string
	active bool
}

func InitPeerRouting() *PeerRouting{
	return &PeerRouting{make(map[string]*peer)}
}

func (p *PeerRouting) GetHeartbeatRequired(addr string) (bool, time.Duration, error){
	fmt.Println("Recieved request of:", addr)
	pr, exist := p.peers[addr]
	fmt.Println()
	if !exist {
		return false, time.Duration(time.Second), fmt.Errorf("NO ENTRY OF PEER");
	}
	timeNextHeartbeatTest:= pr.ttl.Sub(time.Now()).Seconds()
	fmt.Println("time until heartbeat: [", timeNextHeartbeatTest, "ms]")
	if timeNextHeartbeatTest < 1 {
		fmt.Println("Time elapsed -> need to update with heartbeat ping")
		pr.ttl = time.Now().Add(time.Duration(time.Second*HEARTBEAT_FREQ))
		fmt.Println("Time elapsed, setting new TTL for in:", pr.ttl.Sub(time.Now()).Seconds())
		return true, time.Duration(time.Second*HEARTBEAT_FREQ), nil // No need to heartbeat, but we need to check in
	}
	// Time still left until heartbeat.
	return false, time.Duration(timeNextHeartbeatTest), nil
}

func (p *PeerRouting) UpdateTTL(addr string) (error){
	pr, exist := p.peers[addr]
	if exist {
		pr.ttl = time.Now().Add(time.Duration(time.Second*HEARTBEAT_FREQ))
		fmt.Println("NEW TTL SET TO OCCUR IN:",pr.ttl.Sub(time.Now()).Seconds())
		return nil
	}
	return fmt.Errorf("NO ENTRY")
}

func (p *PeerRouting) GetTTL(addr string) (time.Time, error){
	pr, exist := p.peers[addr]
	if exist {
		return pr.ttl, nil
	}
	return time.Now(), fmt.Errorf("NO ENTRY")
}




func (p *PeerRouting) AddPeer(addr string, contact string, hz int) error{
	_, exists := p.peers[addr]
	if exists{
		return fmt.Errorf("ADDR ALRDY EXISTS") 
	}

	p.peers[addr] = &peer{ttl: time.Now().Add(time.Duration(time.Second*HEARTBEAT_FREQ)), auth: "secret key", contact: contact}
	fmt.Println("FOR USER:", addr)
	fmt.Println("ADDED USER: ", p.peers[addr])
	return nil
}




