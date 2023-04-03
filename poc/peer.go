package poc

import(
	"fmt"
	"time"
)

type Peer struct{
	ttl time.Time 
	host string
	active bool
	addr string
	role RaftStatus
	term int
}

// Create peer factory?

const(
	TEMPORARY_LEADER_TTL=5
)

func InitPeer(addr string, hostname string) *Peer{
	return &Peer{
		host: hostname,
		active: false,
		addr: addr,
		role: MEMBER,
		term: 0,
	}
}

func (p *Peer) GetTimeRemaining() time.Duration{
	ttlTime := p.ttl.Sub(time.Now())
	if ttlTime < 0{
		p.deactivatePeer()
	}
	return ttlTime 
}

func (p *Peer) AssignLeader() bool{
	p.role = LEADER	
	p.ttl = time.Now().Add(time.Duration(time.Second*TEMPORARY_LEADER_TTL)) 
	return true
}

func (p *Peer) IsLeader() bool{
	return p.role == LEADER
}

func (p *Peer) IsMember() bool{
	return p.role == MEMBER
}

func (p *Peer) DeactivateLeader() bool{
	p.role = MEMBER
	return true
}

func (p *Peer) GetTerm() int{
	return p.term
}

func (p *Peer) deactivatePeer() {
	p.active = false
	p.role = MEMBER
}

func (p *Peer) ToString() string{
	var roleString string
	var ttlString string
	switch p.role{
	case LEADER:
		roleString = "LEADER"
		ttlString = fmt.Sprintf("%d",p.GetTimeRemaining())
	case CANDIDATE:
		roleString = "CANDIDATE"
		ttlString = "NA"
	default:
		roleString = "MEMBER"
		ttlString = "NA"
	}
	return fmt.Sprintf("[HOST: %s, ROLE: %s, TTL: %s]",p.host, roleString, ttlString)
} 

