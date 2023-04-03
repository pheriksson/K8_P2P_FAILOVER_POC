package poc

import (
	"fmt"
	"log"
	"time"
)

const (
	TIME_RANGE_HEARTBEAT = 1
)

type PeerRouting struct{
	self	*Peer
	table	map[string]*Peer
	active	bool
}

func InitPeerRouting() *PeerRouting{
	return &PeerRouting{InitPeer("NA","SELF"),make(map[string]*Peer), false}
}

func (p *PeerRouting) GetRole() RaftStatus{
	return p.self.role
}

func (p *PeerRouting) GetTerm() int{
	return p.self.GetTerm()
}

func (p *PeerRouting) IsPeer(addr string) bool{
	_, exists := p.table[addr]
	return exists 
}


func (p *PeerRouting) GetMembers() []string{
	peerAddres := make([]string, len(p.table)) 
	for addr, peer := range p.table{
		if peer.IsMember(){
			peerAddres = append(peerAddres, addr)
		}else{
			log.Printf("%s IS NOT MEMBER", peer.ToString())
		}
	}
	return peerAddres
}

func (p *PeerRouting) LockRoutingTableEntries() int{
	p.active = true
	return (len(p.table)+1)/2
}

func (p *PeerRouting) AddPeer(addr string, contact string) error{
	if p.active {
		return fmt.Errorf("CANNOT ADD ENTRIES AFTER PROTOCOL INITIATION")
	}
	_, exists := p.table[addr]
	if exists{
		return fmt.Errorf("ENTRY EXISTS") 
	}
	p.table[addr] = InitPeer(addr, contact)
	return nil
}

func (p *PeerRouting) GetLeaderTimeout() (time.Duration,error){
	//TODO Make sure that only one peer is leader. 
	for _, v := range p.table{
		if v.IsLeader() {
			return v.GetTimeRemaining(), nil	
		}
	} 	
	return time.Duration(0), fmt.Errorf("NO LEADER ELECTED") 
}



func (p *PeerRouting) AssignRandomCandidate() (*Peer ,error){
	if !p.self.IsLeader(){
		p.self.role = MEMBER
		return &Peer{}, fmt.Errorf("CANNOT ASSIGN CANDIDATE AS NON LEADER")
	}	
	for _, peer := range p.table{
		if peer.IsMember(){
			return peer, nil
		}
	}
	return &Peer{}, fmt.Errorf("NO PEER ENTRY")
}



func (p *PeerRouting) GetDisconnectLeader() (bool, time.Duration, error){
	timeNextHeartbeat, err := p.GetLeaderTimeout()
	if err != nil {
		return true, time.Duration(0), err 
	}

	// TODO: Dependent on co routine runtime will pass timer or just about repeat until
	// time has 'time' to update, meaning might get 5k calls with heartbeat ttl of < 1ns otherwise. 
	if timeNextHeartbeat < TIME_RANGE_HEARTBEAT {
		return true, time.Duration(0), nil 
	}
	return false, time.Duration(timeNextHeartbeat), nil
}


//NOTE Do leader only.
func (p *PeerRouting) UpdateTTL(addr string) (error){
	pr, exist := p.table[addr]
	if exist {
		log.Println("TRYING TO UPDATE TTL FOR: ",pr.ToString())
		pr.ttl = time.Now().Add(time.Duration(time.Second*HEARTBEAT_FREQ))
		pr.active = true
		return nil
	}
	return fmt.Errorf("NO ENTRY")
}


func (p *PeerRouting) BecomeMember(newTerm int){
	p.self.role = MEMBER
	p.self.term = newTerm 
} 


func (p *PeerRouting) BecomeLeader(){
	p.self.role = LEADER
	for _, peer := range p.table{
		peer.role = MEMBER
	}
}

