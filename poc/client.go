package poc

import (
	"fmt"
	"log"
	"net"
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
	leaderVote	chan int
	quorum int
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
			// TODO: At time T, send to candidate payload.
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
			// No leader -> Try and become leader by advanding term by 1.  
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
	// NOTE: Might have to empty leaderVote channel here.
	// TODO Insert mutex to lock channel write to leaderVote until timeout/newterm/or quorum
	//mutex channelLeaderVote unlock. -> and when trying to write to channel, make sure that the term is the same in package as current term -> success.
	//defer mutex channelLeaderVote lock.
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
			// Someone else started vote for leader.
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
			// TODO: Make sure that channel is written to incase of new term.
			// Update term in receiving packages (as we will aknowledge).
			log.Println("NEW TERM AKNOWLEDGED - ", newTerm)
			// NOTE: Incoming package should update and set self to become member.
			return false, nil
		}

	}
}

// TODO: Make sure this is used by LEADER.
// TODO: Ping every peer. 
// TODO: Read channel for new term -> (healing from partition -> new cuorum).
func (p *PoC) startHeartbeat(heartbeat time.Duration){
	go func(){
		members := p.peers.GetMembers()
		for{
			select{
			case <-time.After(heartbeat):
				for _, addr := range members{
					fmt.Printf("SIMULATING PING TO: %s",addr)
				}
			case <-p.oldTerm:
				// NOTE -> NEED TO REGISTER NEW TERM BEFORE SENDING TO CHANNEL OLD TERM AND ASSIGN SELF TO MEMBER
				log.Printf("REGISTERED NEW TERM OF: %d", p.peers.GetTerm())
				return
			}
		}
	}()
}

// TODO: This is to be used by CANDIDATE, MEMBER, simply await for timeout and refresh with new ttl after each timeout timer.
// TODO: READ NEW TERM CHANNEL ASWELL - Partition healing, if not and both leader and this simply awaits, no self healing.
// So have to react to new terms from either methods. 
func (p *PoC) timeoutLeader(heartbeatTimeout time.Duration){
	//newLeaderPacket := make(chan struct{}) // Have this channel written to incase of new leader selection registered 
	go func(){
		select{
		case <- time.After(heartbeatTimeout):
			// heartbeatTimeout -> get new heartbeat incase of incoming ttl packet by leader. 		
			timeout, nextHeartbeatTimeout, err := p.peers.GetDisconnectLeader()
			if err != nil{
				// Leader unknown/not registered. 
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
	// Currently treaing any incoming rpc as a heartbeat.
	// Call to updateTTL for user at packet and set their status to active.
	// On new term -> send to channel of new term.
	// On package from leader -> increase TTL.
	// Else, just perform action. 
	// Ok RPC we will respond to. Any package will update the peers TTL (as long as it exists within the route). 

	callerAddrs := packet.Caller.IP.String()
	fmt.Println("Recieved packet from:", callerAddrs)
	// Check that peer is within allowed routing.
	if !p.peers.IsPeer(callerAddrs){
		log.Printf("UNKNOWN PEER REQUEST - %s", callerAddrs)
		return
	}
	// Decode packet to corresponding rpc.
	

	go func(){
		p.leaderVote<-1
	}()
	fmt.Println("WRITEN TO CHANNEL");
	p.peers.UpdateTTL(packet.Caller.IP.String())
}


func (p *PoC) StartPoc(){
	go p.net.Listen()
	p.registerPeer("127.0.0.2", "TESTING - PEER1")
//	p.registerPeer("127.0.0.3", "TESTING - PEER2")
//	p.registerPeer("127.0.0.4", "TESTING - PEER3")
//	p.registerPeer("127.0.0.5", "TESTING - PEER4")
//	p.registerPeer("127.0.0.6", "TESTING - PEER5")
//	p.registerPeer("127.0.0.7", "TESTING - PEER6")

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
	p.net.SendRequest(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port} , []byte(msg))
}


