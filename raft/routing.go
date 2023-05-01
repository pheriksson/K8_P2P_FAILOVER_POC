package raft

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



func InitPeerRouting(hostAddress string) *PeerRouting{
	return &PeerRouting{InitPeer(hostAddress,"SELF"),make(map[string]*Peer), false}
}

func (p *PeerRouting) GetAddr() string{
	return p.self.addr
}


func (p *PeerRouting) GetRole() RaftStatus{
	return p.self.role
}

func (p *PeerRouting) GetRoleString() string{
	switch (p.self.role){
	case LEADER:
		return "LEADER"
	case CANDIDATE:
		return "CANDIDATE"
	case MEMBER:
		return "MEMBER"
	default:
		return "UNKNOWN"
	}

}

func (p *PeerRouting) GetTerm() int{
	return p.self.GetTerm()
}

func (p *PeerRouting) IsPeer(addr string) bool{
	_, exists := p.table[addr]
	return exists 
}

func (p *PeerRouting) GetPeersAddress() []string{
	peerAddres := make([]string, len(p.table)) 
	for addr,_ := range p.table{
		peerAddres = append(peerAddres, addr)
	}
	return peerAddres
}

func (p *PeerRouting) GetPeers() []*Peer{
	peers := make([]*Peer, len(p.table))
	for _, p := range p.table{
		peers = append(peers, p)
	}
	return peers
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

func (p *PeerRouting) GetLeaderAddr() (string, error){
	if p.self.IsLeader() {
		return p.self.GetAddr(), nil
	}
	for addr, peer := range p.table{
		if peer.IsLeader() {
			return addr, nil 
		}	
	}
	return "", fmt.Errorf("NO ENTRY")
}


func (p *PeerRouting) GetLeaderTimeout() (time.Duration,error){
	//TODO Make sure that only one peer is leader. 
	for _, v := range p.table{
		if v.IsLeader() {
			leaderTTL := v.GetTimeRemaining()
			if leaderTTL < 0{
				return time.Duration(0), fmt.Errorf("LEADER TIMEOUT")
			}
			return leaderTTL, nil	
		}
	} 	
	return time.Duration(0), fmt.Errorf("NO LEADER ELECTED") 
}

func (p *PeerRouting) GetCandidateAddress() (string, error){
	for addr, peer := range p.table{
		if peer.IsCandidate(){
			return addr, nil
		}
	}
	return "", fmt.Errorf("NO ASSIGNED CANDIDATE")
}

func (p *PeerRouting) GetRandomMember() (*Peer ,error){
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

func (p *PeerRouting) CheckLeaderTimeout() (bool, time.Duration, error){
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
		pr.ttl = time.Now().Add(time.Duration(time.Second*SEC_TIME_TIMEOUT_LEADER_HB))
		// Starting traffic - making sure no new peers can be added.
		pr.active = true
		return nil
	}

	return fmt.Errorf("NO ENTRY")
}

func (p *PeerRouting) RaiseTerm(increase int){
	if increase > 0{
		p.self.RaiseTerm(increase)
	}

}

func (p *PeerRouting) LowerTerm(decrease int){
        if decrease > 0 {
            p.self.LowerTerm(decrease)
	}
}

func (p *PeerRouting) BecomeMember(newTerm int) bool{
	p.self.term = newTerm 
	if !p.self.IsMember(){
		p.self.role = MEMBER
		return true
	}
	return false
} 

func (p *PeerRouting) BecomeCandidate() bool{
	defer func(){p.self.role = CANDIDATE}()
	if (p.self.IsMember()){
		return true
	}
	return false
}

// Refactor - zz
func (p *PeerRouting) MakeCandidate(addr string) bool{
	newCandidateAssigned := false
	for paddr, peer := range p.table{
		if peer.IsCandidate() && paddr != addr{
			peer.role = MEMBER
		}
		if (paddr == addr){peer.role = CANDIDATE; newCandidateAssigned = true}
	}
	if p.self.GetAddr() == addr && !p.self.IsCandidate(){
		p.self.role = CANDIDATE
		return true
	}else if p.self.IsCandidate() && newCandidateAssigned{
		p.self.role = MEMBER
	}
	return newCandidateAssigned
}

func (p *PeerRouting) BecomeLeader(){
	p.self.role = LEADER
	for _, peer := range p.table{
		peer.role = MEMBER
	}
}

func (p *PeerRouting) WipeLeader(){
	p.self.role = MEMBER
	for _, peer := range p.table{
		peer.role = MEMBER
	}
}

func (p *PeerRouting) MakeLeader(addr string) bool{
	peer, exist := p.table[addr]
	if exist{
		peer.role = LEADER
		log.Println(addr,"ASSIGNED AS LEADER")
		return true
	}
	if p.self.GetAddr() == addr{
		p.self.role = LEADER
		log.Println("SELF ASSIGNED AS LEADER")
		return true
		
	}
	log.Println("NOT ASSIGNED AS LEADER, addr:",addr)
	return false
}



