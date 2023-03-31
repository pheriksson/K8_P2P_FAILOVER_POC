package poc

import (
	"fmt"
	"time"
)

const (
	TIME_RANGE_HEARTBEAT = 1
)

type PeerRouting struct{
	peers	map[string]*peer 
}

type peer struct{
	ttl time.Time 
	auth string // not used. 
	contact string
	active bool
	addr string
}

func InitPeerRouting() *PeerRouting{
	return &PeerRouting{make(map[string]*peer)}
}

func (p *PeerRouting) GetHeartbeatRequired(addr string) (bool, time.Duration, error){
	pr, exist := p.peers[addr]
	if !exist {
		return false, time.Duration(time.Second), fmt.Errorf("NO ENTRY OF PEER");
	}
	timeNextHeartbeat:= pr.ttl.Sub(time.Now())
	// TODO: Dependent on co routine runtime will pass timer or just about repeat until
	// time has 'time' to update, meaning might get 5k calls with heartbeat ttl of < 1ns otherwise. 
	if timeNextHeartbeat < TIME_RANGE_HEARTBEAT {
		return true, time.Duration(time.Second*HEARTBEAT_FREQ), nil 
	}
	// Time still left until heartbeat ( > TIME_RANGE_HEARTBEAT).
	return false, time.Duration(timeNextHeartbeat), nil
}

func (p *PeerRouting) UpdateTTL(addr string) (error){
	pr, exist := p.peers[addr]
	if exist {
		pr.ttl = time.Now().Add(time.Duration(time.Second*HEARTBEAT_FREQ))
		pr.active = true
		return nil
	}
	return fmt.Errorf("NO ENTRY")
}

func (p *PeerRouting) ValidTTL(addr string) (bool, error){
	pr, exists := p.peers[addr]
	if !exists {
		return false, fmt.Errorf("NO ENTRY")
	}
	return (pr.ttl.Sub(time.Now()) > 0), nil
}

func (p *PeerRouting) DeactivatePeer(addr string){
	pr, exists := p.peers[addr]
	if !exists {
		return
	}
	pr.active = false
}

func (p *PeerRouting) AddPeer(addr string, contact string, hz int) error{
	_, exists := p.peers[addr]
	if exists{
		return fmt.Errorf("ENTRY EXISTS") 
	}
	p.peers[addr] = &peer{ttl: time.Now().Add(time.Duration(time.Second*HEARTBEAT_FREQ)), auth: "secret key", contact: contact, addr: addr}
	return nil
}




