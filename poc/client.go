package poc

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pheriksson/K8_P2P_FAILOVER_POC/kube"
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/network"
)

const (
	HEARTBEAT_FREQ = 5
	HEARTBEAT_TIMEOUT_VALID_MULTIPLIER = 2 // Num HEARTBEAT_FREQ is allowed to pass until peer is deemed inactive. 	
	LEADER_REQUEST_TIMEOUT_SECONDS = 10
	TIME_SEC_AWAIT_CANDIDATE_CONFIRMATION = 2
)

type RaftStatus int

const (
	LEADER RaftStatus = iota
	CANDIDATE
	MEMBER
)

type PoC struct{
	kube		*kube.KubeClient	
	kubeChan	chan string
	net		*network.Network
	netChan		<-chan network.Packet
	cli		string
	cliChan		<-chan	string 
	peers		*PeerRouting
	role		RaftStatus	
	// Catch all chan used to catch any further terms.
	oldTerm		chan int
	oldTermMut	sync.Mutex
	leaderVote	chan int
	quorum		int

}


func InitPoC(ip string, port int) *PoC{
	nch := make(chan network.Packet)
	n := network.InitNetwork(ip, port, nch)
	p := PoC{kubeChan: make(chan string),
		net: n,
		netChan: nch,
		peers: InitPeerRouting(),
		role: MEMBER,
		oldTerm: make(chan int),
		oldTermMut: sync.Mutex{},
		leaderVote: make(chan int),
		
	}
	return &p
}



func (p *PoC) registerPeer(addr string, contact string){
	err := p.peers.AddPeer(addr, contact)
	if err != nil{
		log.Printf("FAILED TO ADD ENTRY [%s:%s]", addr, contact)
		return
	}
}

func (p *PoC) startModifiedRAFT(){
	p.quorum = p.peers.LockRoutingTableEntries()
	log.Printf("STARTING RAFT AS: %s, QUORUM AT: %d", p.peers.self.ToString(), p.quorum)
	for{
	switch (p.peers.GetRole()){
	case LEADER:
		log.Println("SELF AS LEADER")
		statusLeaderInit, err := p.initLeaderRole()
		if err != nil{
			log.Printf("FAILED SELF LEADER INITIATION. REASON: %s",err)
		}
		p.startHeartbeat(HEARTBEAT_FREQ*time.Second)
		if statusLeaderInit {
			// TODO: START PINGING ALL MEMBERS.
			// TODO: At time T, send volumes to candidate payload.
		}else{
			log.Println("FAILED LEADER SELECTION - continuing as MEMBER")
		}
	case CANDIDATE:
		log.Println("SELF AS CANDIDATE")
		leaderExistError := p.getLeaderTTL()
		if leaderExistError != nil{
			// No leader -> Become leader and advance term by 2 (to counter members term by 1).
			p.requestLeaderVotes(p.peers.self.term+2)	
		}
	case MEMBER:
		log.Println("SELF AS MEMBER")
		leaderExistError := p.getLeaderTTL()
		if leaderExistError != nil{
			// No leader -> Try and become leader by advancing term by 1.  
			p.requestLeaderVotes(p.peers.self.term+1)
		}
	default:
		log.Panic("UNKNWON STATE FOR SELF")
	}
	}	

}

func (p *PoC) requestLeaderVotes(newTerm int) bool{
	log.Printf("PREV TERM: %d\nSTARTING LEADER FOR TERM: %d", p.peers.GetTerm(), newTerm)
	numVotes := 0
	for{
		select{
		case <-p.leaderVote:
			numVotes+= 1
			log.Println("RECIEVED ANOTHER VOTE, TOTAL:", numVotes) 
			if (numVotes >= p.quorum){
				// Become leader and return.
				log.Println("SELF BECAME LEADER BY UNANOMOUS VOTE")
				p.peers.BecomeLeader()
				return true
			}
		case newTerm := <-p.oldTerm:
			log.Println("RECIEVED NEW TERM - ABORTING LEADER ELECTION:", newTerm)
			return false
		case <-time.After(time.Duration(time.Second*LEADER_REQUEST_TIMEOUT_SECONDS)):
			log.Printf("TIMEOUT LEADER ELECTION. RECIEVED (%d/%d) VOTES",numVotes,p.quorum)		
			return false
		}
	}
}


func (p *PoC) getLeaderTTL() error{
	leaderTTL, err := p.peers.GetLeaderTimeout()
	if err != nil{
		return fmt.Errorf("NO LEADER ELECTED IN TERM: %d", p.peers.self.term)
	}else{
		// TODO: Start await for leader timeout
		log.Println("START TO AWAIT LEADER TIMEOUT - ", leaderTTL)
		p.timeoutLeader(leaderTTL)				
		return nil
	}
}

func (p *PoC) initLeaderRole() (bool, error){
	for{
		cand, err := p.peers.AssignRandomCandidate()
		if err != nil{
			return false, fmt.Errorf("CANNOT ASSIGN SELF AS LEADER AND CANDIDATE - %s", err)	
		}
		// Assign candidate - await confirmation from candidate.
		// TODO: p.net.SendCandidateAssignement(cand)
		select{
		case <- time.After(time.Second*TIME_SEC_AWAIT_CANDIDATE_CONFIRMATION):
			// No confirmation, simply pass - will most likely timeout before having another go for candidate. 
			log.Printf("Failed confirmation for candidate from:%s.\nTrying new candidate", cand.ToString())
		case candConfirmation := <-p.netChan:
			log.Println("Recieved confirmation from candidate.",cand.ToString())
			log.Println("CAND-CONFIRMATION",candConfirmation)
			return true, nil
		case newTerm := <-p.oldTerm:
			log.Println("NEW TERM AKNOWLEDGED - ", newTerm)
			return false, nil
		}

	}
}

func (p *PoC) startHeartbeat(heartbeat time.Duration){
	go func(){
		members := p.peers.GetMembers()
		for{
			select{
			case <-time.After(heartbeat):
				for _, addr := range members{
					//TODO Send ping to all members.
					fmt.Printf("SIMULATING PING TO: %s",addr)
				}
			case <-p.oldTerm:
				log.Printf("REGISTERED NEW TERM OF: %d", p.peers.GetTerm())
				return
			}
		}
	}()
}

func (p *PoC) timeoutLeader(heartbeatTimeout time.Duration){
	go func(){
		select{
		case <- time.After(heartbeatTimeout):
			timeout, nextHeartbeatTimeout, err := p.peers.GetDisconnectLeader()
			if err != nil{
				log.Printf("UNKNOWN LEADER. %s", err)
				return
			}
			if timeout {
				log.Printf("LEADER TIMEOUT OCCURED")
				return 
			}
			log.Printf("AWAITING NEXT HEARTBEAT IN %d", nextHeartbeatTimeout)
			p.timeoutLeader(nextHeartbeatTimeout)
		case <-p.oldTerm:
			log.Println("NEW TERM REACHED")
		}	

	}()
}

// Keep hash log of volumes as commit log?
func (p *PoC) handleRequest(packet network.Packet){
	callerAddrs := packet.Caller.IP.String()
	if !p.peers.IsPeer(callerAddrs){
		log.Printf("UNKNOWN PEER REQUEST - %s", callerAddrs)
		return
	}
	p.peers.UpdateTTL(callerAddrs)
	// TODO: Refactor
	checkTerm  := func(t int){
		if p.checkTerm(t){
			//Term reached, default to member
			return
		}
	}
	switch packet.Type{
	case network.RAFT_VOTE:
		val, err := decodePayload[Vote](packet.Data)	
		checkTerm(val.Term)
		go func(){
			log.Println("RECIEVED: ", val, err)
		}()
	case network.RAFT_REQUEST_VOTE:
		val, err := decodePayload[RequestVote](packet.Data)	
		checkTerm(val.Term)
		go func(){
			log.Println("RECIEVED: ", val, err)
		}()
	case network.REQUEST_PERSISTENT_VOLUMES:
		val, err := decodePayload[RequestVolumes](packet.Data)	
		checkTerm(val.Term)
		go func(){
			log.Println("RECIEVED: ", val, err)
		}()
	}
}

func (p *PoC) checkTerm(incTerm int) bool{
	p.oldTermMut.Lock()
	defer p.oldTermMut.Unlock()
	if incTerm > p.peers.GetTerm(){
		p.peers.BecomeMember(incTerm)
		p.oldTerm <- incTerm
		return true 
	}
	return false
}


func (p *PoC) StartPoc(){
	go p.net.Listen()
	p.registerPeer("127.0.0.2", "TESTING - PEER1")
	go p.startModifiedRAFT()
	go func(){
		time.Sleep(time.Second*2)
		fmt.Println("SENDING REQUEST.")
		p.TestSendLocalRequest(9999,"testing")
		time.Sleep(time.Second*5)	
		go func(){
		}()

	}()
	t := 0
	for{
		select{
		case incPacket := <-p.netChan:
			go p.handleRequest(incPacket)
		case <-time.After(time.Second):
			t+=1	
			fmt.Println("",t,"(S) PASSED IN MAIN POC")
		}

	}
}

func (p *PoC) TestSendLocalRequest(port int, msg string){
	test, _ := encodePayload(RequestVolumes{2})
	p.net.SendRequest(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port} , test)
}


