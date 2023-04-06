package poc

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/pheriksson/K8_P2P_FAILOVER_POC/kube"
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/network"
)

const (
	HEARTBEAT_FREQ = 5
	HEARTBEAT_TIMEOUT_VALID_MULTIPLIER = 5 
	TTL_TIMEOUT_SECONDS = HEARTBEAT_FREQ*HEARTBEAT_TIMEOUT_VALID_MULTIPLIER
	TIME_SEC_SEND_LEADER_ELECTION = 5
	// TTL_TIMEOUT_SECONDS = Num pings send by leader before timeout is valid
	TIME_SEC_AWAIT_CANDIDATE_CONFIRMATION = 2
	TIME_SEC_SEND_VOLUME_CANDIDATE = 20
	TESTING_PORT = 9999
	TIME_SEC_AWAIT_CANDIDATE_RESPONSE = 10
	MISSING_CANDIDATE_STRING = "TBD"
	RANDOM_NUMBER_RANGE = 6000
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
	candidatePromotion chan bool
	candidatePromotionConfirmed chan string
	candidateVolumeConfirmation chan bool
	startEarlyFailover chan bool
	runningElectionMut sync.Mutex
	runningElection bool
	quorum		int
	r	int 
}


func InitPoC(ip string, port int) *PoC{
	nch := make(chan network.Packet)
	n := network.InitNetwork(ip, port, nch)
	p := PoC{kubeChan: make(chan string),
		net: n,
		netChan: nch,
		peers: InitPeerRouting(ip),
		role: MEMBER,
		oldTerm: make(chan int),
		oldTermMut: sync.Mutex{},
		leaderVote: make(chan int),
		candidatePromotion: make(chan bool),
		candidatePromotionConfirmed: make(chan string),
		candidateVolumeConfirmation: make(chan bool),	
		startEarlyFailover: make(chan bool),
		runningElectionMut: sync.Mutex{},
		runningElection : false,
		r: rand.Intn(RANDOM_NUMBER_RANGE),
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
	p.roleLog("STARTING MODIFIED RAFT AS:"+p.peers.self.ToString()+", QUORUM AT:"+strconv.Itoa(p.quorum))
	for{
	switch (p.peers.GetRole()){
	case LEADER:
		// TODO: Need to init kubernetes cluster and have appropriate channel to early failover to candidate. 
		p.roleLog("SELF AS LEADER")
		newTerm := make(chan bool) 
		// volume can terminate HB on loss of candidate ping and if no candidatePromoti is confirmed -> size 1 for no DL
		termHeartbeat := make(chan bool, 1) // Channel to terminate heartbeat.
		go p.startHeartbeat(HEARTBEAT_FREQ*time.Second, newTerm, termHeartbeat)
		confirmedCandidate := p.assignCandidate(newTerm)
		if confirmedCandidate{
			p.startVolumeWindowCandidate(newTerm, termHeartbeat)
		}
		termHeartbeat <- true
	case CANDIDATE:
		// TODO: Case of early failover - Leader msg of recovery from latest volume(s).
		p.roleLog("SELF AS CANDIDATE")
		noValidLeader := p.watchLeader()
		if noValidLeader != nil{
			// No leader -> Become leader and advance term by 2 (to counter members term by 1).
			p.requestLeaderVotes(2)	
		}
	case MEMBER:
		p.roleLog("SELF AS MEMBER")
		noLeaderError := p.watchLeader() 
		if noLeaderError != nil{
			// No leader -> Try and become leader by advancing term by 1.  
			p.requestLeaderVotes(1)
		}
	default:
		log.Panic("UNKNWON STATE FOR SELF")
	}
	}	

}


func (p *PoC) requestLeaderVotes(incTerm int) bool{
	p.roleLog("STARTING LEADER FOR TERM:+"+strconv.Itoa((p.peers.GetTerm()+incTerm)))
	p.peers.RaiseTerm(incTerm)
	numVotes := 0
	members := p.peers.GetPeersAddress()
	for{
		// Send out all request replies.
		p.sendRequestLeaderVotes(members, p.peers.GetTerm())
		select{
		case <-p.leaderVote:
			numVotes+= 1
		        p.roleLog("RECIEVED ANOTHER VOTE, TOTAL:"+strconv.Itoa(numVotes))
			if (numVotes >= p.quorum){
				p.peers.BecomeLeader()
				p.roleLog("NEW ROLE: LEADER")
			        // Bootstrap one health check to init leader role.
			        p.sendLeaderPings(p.peers.GetPeersAddress())
				return true
			}
		case<-p.oldTerm:
		         p.roleLog("ABORTING LEADER ELECTION ON SELF - new term from update")
			return false
		case <-time.After(time.Duration(time.Second*TIME_SEC_SEND_LEADER_ELECTION)):
		        p.roleLog("TIMEOUT LEADER ELECTION. RECIEVED ("+strconv.Itoa(numVotes)+"/"+strconv.Itoa(p.quorum)+")") 
		        p.peers.LowerTerm(incTerm) 
	                p.roleLog("RANDOM SLEEP TO AWAIT ELECTION/PART HEALING")
	                time.Sleep(time.Duration(p.r)*time.Millisecond)
			return false
		}
	}
}

// Watch for leader activity - return on inactive leader/promotion to candidate.
func (p *PoC) watchLeader() (error){
	p.roleLog("TRYING TO WATCH LEADER")
	leaderTTL, err := p.peers.GetLeaderTimeout()
	if err != nil{
		p.roleLog("NO VALID LEADER ELECTED.")
		return fmt.Errorf("NO VALID LEADER ELECTED IN TERM: %d.\nREASON: %s", p.peers.GetTerm(), err)
	}
	p.roleLog("START TO AWAIT LEADER TIMEOUT."+strconv.Itoa(int(leaderTTL.Seconds())))
	// Will return on timeout / member becoming candidate / new term 
	p.listenLeader(leaderTTL)				
	return nil
}

// Will return if no heartbeat occurs within leader TTL OR member promotion. 
func (p *PoC) listenLeader(heartbeatTimeout time.Duration){
	select{
	case <- time.After(heartbeatTimeout):
		timeout, nextHeartbeatTimeout, err := p.peers.CheckLeaderTimeout()
		if err != nil || timeout{
			p.roleLog("LEADER TIMEOUT")	
			return 
		}
		p.roleLog("HB EXPECTED IN:"+nextHeartbeatTimeout.String())
		p.listenLeader(nextHeartbeatTimeout)
	case <-p.oldTerm:
		p.roleLog("NEW TERM REACHED")
		if p.peers.BecomeMember(p.peers.GetTerm()){
			p.roleLog("DEMOTED TO MEMBER")
		}
		return
	case <- p.startEarlyFailover:
		p.roleLog("<<---- TODO: INSERT EARLY FAILOVER")
		if (p.peers.GetRole() == CANDIDATE){
			p.awaitKubernetesCluster()
			return
		}
	}	
}

func (p *PoC) awaitKubernetesCluster(){
	p.kubeChan<-"START_CLUSTER"
	for{
		select{
		case <-time.After(time.Second):
			log.Println("Awaiting k8 cluster takeover")	
		case <- p.kubeChan:
			log.Println("Got confirmation of cluster. Taking leader position.")
			return
		}
	}
}



// Will only return on succesfull candidate confirmation or signal from heartbeat call that new term reached. 
func (p *PoC) assignCandidate(abort chan bool) bool{
	allPeers := p.peers.GetPeersAddress()
	for _, peer := range allPeers{
		if (peer == ""){continue}
		p.roleLog("SEND BECOME CANDIDATE TO CAND:"+peer)
		p.sendRequestCandidateVote(peer) 
		select{
		case <-time.After(time.Second*TIME_SEC_AWAIT_CANDIDATE_RESPONSE):
		        p.roleLog("FAILED TO GET CONFIRMATION FROM: "+peer)	
		case confirmed :=<-p.candidatePromotionConfirmed:
			if confirmed == peer{
		                // Here bug with two members believign they're candidates.
				p.roleLog("GOT CANDIDATE CONFIRMATION FROM: "+confirmed)
				p.peers.MakeCandidate(confirmed)
				return true	
			}
		case <-abort:
			log.Println("Aborting candidate selection due to hearbeat notification of new term.")
			return false
		}
	}
	p.roleLog("CYCLED THROUGH ALL MEMBERS, RESTARTING CYCLING.")
	return p.assignCandidate(abort)

}


// TODO: Unhealthy cluster. Notfiy leader of promoting candidate.
func (p *PoC) startHeartbeat(heartbeat time.Duration, stopVolumeRpc chan<- bool, stopHeartbeat <-chan bool){
	members := p.peers.GetPeersAddress()
	for{
		select{
		case <-time.After(heartbeat):
			p.sendLeaderPings(members)
		case <-p.oldTerm:
			log.Printf("REGISTERED NEW TERM OF: %d", p.peers.GetTerm())
			stopVolumeRpc<-true 
			return
		case <-stopHeartbeat:
			log.Println("HEARBEAT - RECIEVED STOP FROM LEAVING LEADER STATE")		
			return
	}
	}
}

func (p *PoC) startVolumeWindowCandidate(stopFromHeartbeat chan bool, stopHeartbeat chan bool){
	p.roleLog("INITIATING PERSISTENT VOLUME WINDOW TRANSFER")
	for{
		select{
		case <-time.After(time.Second*TIME_SEC_SEND_VOLUME_CANDIDATE):
			candAddr, err := p.peers.GetCandidateAddress()
			if err != nil{
				// Invalid leader state - missing candidate. 
				p.roleLog("INVALID STATE: MISSING CANDIDATE")
				stopHeartbeat<-true	
				return
			}
			p.roleLog("TODO: SEND VOLUME TO CANDIDATE:"+ candAddr)	
			select{
				case <-time.After(time.Second*TIME_SEC_AWAIT_CANDIDATE_RESPONSE):
					p.roleLog("TIMEOUT RESPONSE OF VOLUME - ASSUMING CANDIDATE DEAD")
					stopHeartbeat<- true
					return
				case <-p.candidateVolumeConfirmation:
					p.roleLog("RECIEVED CAND CONFIRMATION OF VOLUME")
			}	
		case <-stopFromHeartbeat:
			// New term recieved.
			return
		}
	}

}

// REFACTOR: send to altered raft api if packet labeled as raft rpc. 
func (p *PoC) handleRequest(packet network.Packet){
	callerAddrs := packet.Caller.IP.String()
	if !p.peers.IsPeer(callerAddrs){
		p.roleLog("UNKNOWN PEER REQUEST - ["+callerAddrs+"]"+" PACKET TYPE:"+packet.Type.ToString())
		return
	}
	// TODO: BUG when leader joins quickly from disconnecting (as member), will start to ping for election ()
	// And other will register as current leader pinging, but infact prev leader is out of sync and dead (as in member starte).
	// Can be recovered by candidate pinging leader for confirmation, or simply move updateTTL for every request.
	err := p.peers.UpdateTTL(callerAddrs)
	if err != nil{
		p.roleLog("FAILED TO UPDATE TTL FOR :"+callerAddrs+ " (NO ENTRY)")
	}
	switch packet.Type{
	case network.RAFT_VOTE_LEADER:
		val, _ := decodePayload[VoteLeader](packet.Data)	
		// Cases.
		// 1, new term -> new leader, reply with agreed and await ().
		// 2, same term/old term -> simply reply with current term and addr of leader.
		go func(){
			log.Println()
			p.roleLog("RAFT_VOTE_LEADER - RECIEVED: "+fmt.Sprintf("%#v",val))

			if p.checkTerm(val.Term, val.LeaderAddr){
				// New term, meaning only possible msg is a request to become leader.
				// If we we're in leader selection, send signal to abort leader
                                p.roleLog("HIGHER TERM")
				p.sendReplyLeaderVote()
				return
			}
			if val.Term == p.peers.GetTerm() && val.Agreed && val.LeaderAddr == p.peers.GetAddr() {
				// Same term/ Agreed / Packet has self as leader -> we're requesting the vote.
				p.roleLog("AS MEMBER - GOT VOTE.")
				p.leaderVote <- 1
				return
			}
			// Case same term or lower -> if same term, and val agreed == false (they're requesting we vote for them).
			// Same term but caller is candidate -> accept vote. 
			candAddr, err := p.peers.GetCandidateAddress()
			if err == nil && val.Term == p.peers.GetTerm() && p.peers.self.IsMember() && !val.Agreed && val.LeaderAddr == candAddr{ 
                                p.roleLog("ASS MEMBER, GOT REQUEST TO VOTE ON PREVIOUSLY CONFIRMED CANDIDATE IN SAME TERM. -> ABORT OWN REQ LEADER.")
				p.peers.MakeLeader(val.LeaderAddr)
				p.oldTerm<-val.Term
				p.sendReplyLeaderVote()
				return
			}
		}()
	case network.RAFT_VOTE_CANDIDATE:
		val, err := decodePayload[VoteCandidate](packet.Data)
		if err != nil{
			log.Println("RAFT_VOTE_CAND ERROR:", err)
			return
		}
		go func(){
			p.roleLog("RAFT_VOTE_CANDIDATE - RECIEVED: "+fmt.Sprintf("%#v",val))
			if p.checkTerm(val.Term, val.LeaderAddr){
				// No write to candidate promotion as oldTerm channel arldy written to by checkterm. 
				p.roleLog("RECIEVED REQUEST TO BECOME CANDIDATE ON NEWER TERM - ACCEPT.")
				p.sendReplyCandidateVote()
			}
			if (val.Term == p.peers.GetTerm()){
				// Same term, leader requesting candidate, or leader recieving response.  
				lAddr, err := p.peers.GetLeaderAddr()
				if val.LeaderAddr == lAddr && err == nil && !val.Agreed{ 
					// Candidate responding on same term
					p.sendReplyCandidateVote()
					return
				}
				if lAddr == p.peers.GetAddr() && val.Agreed{
					// Leader recieving confirmation
					p.candidatePromotionConfirmed <- val.CandidateAddr
				}
			}
		}()
	case network.RAFT_HEALTH:
		val, err := decodePayload[Health](packet.Data)	
		if err != nil{
			log.Println("RAFT_HEALTH ERROR PACKET FROM: ",err)
			return
		}
		go func(){
			p.roleLog("RAFT_HEALTH - RECIEVED: "+fmt.Sprintf("%#v",val))
			// NOTE: Ordering matters here, p.checkTerm needs to be on the LHS of OR expression.
			if (p.checkTerm(val.Term, val.LeaderAddr) || val.Term == p.peers.GetTerm()){
				if (p.peers.MakeCandidate(val.CandidateAddr)){
					p.roleLog("NEW ADDRESS REGISTERED FOR: CANDIDATE")
				}
				if (!val.Stable && p.peers.GetRole() == CANDIDATE){
					p.roleLog("UNSTABLE LEADER - BECOME LEADER EARLY.")
					p.startEarlyFailover <- true
				}
			}
		}()
	}
}

func (p *PoC) checkTerm(newTerm int, leaderAddr string) bool{
	p.oldTermMut.Lock()
	defer p.oldTermMut.Unlock()
	if newTerm > p.peers.GetTerm(){
		p.peers.BecomeMember(newTerm)
		p.peers.MakeLeader(leaderAddr)
		p.roleLog("NEW TERM:"+strconv.Itoa(newTerm))
		p.roleLog("NEW LEADER:"+leaderAddr)
		p.oldTerm <- newTerm
		return true 
	}
	return false
}


//
func (p *PoC) sendLeaderPings(peers []string){
	leaderAddr := p.peers.GetAddr()
	candidateAddr, err := p.peers.GetCandidateAddress()
	if err != nil {
		candidateAddr = MISSING_CANDIDATE_STRING 
	}
	rpcPayload, err := encodePayload(Health{
		Term: p.peers.GetTerm(),
		LeaderAddr: leaderAddr,
		CandidateAddr: candidateAddr,
		Stable: true,
	})
	if err != nil {
		log.Println("FAILED TO ENCODE LEADER REPLY")
	}
	for _, peer := range peers{
		if peer == "" {continue}
		p.roleLog("SENDING PING TO: "+peer)
		p.sendRequest(peer, rpcPayload, network.RAFT_HEALTH)	
	}
}

func (p *PoC) sendRequestLeaderVotes(peers []string, newTerm int){
	rpcPayload, err := encodePayload(VoteLeader{
		Term: newTerm,
		LeaderAddr: p.peers.GetAddr(),
		Agreed: false,
	})
	if err != nil{
		log.Println("FAILED TO ENCODE LEADER REQUEST")
	}
	for _, peer := range peers{
		if peer == "" {continue}
		p.roleLog("SENDING VOTE REQUEST TO:"+peer)
		p.sendRequest(peer, rpcPayload, network.RAFT_VOTE_LEADER)
	}
}

func (p *PoC) sendReplyLeaderVote() {
	leaderAddr, err := p.peers.GetLeaderAddr() 
	p.roleLog("SEND REPLY LEADER VOTE, CURRENT LEADER:"+leaderAddr)
	if err != nil{
		p.roleLog("NO LEADER TO BE FOUND"+err.Error())
	}
	rpcPayload, err := encodePayload(VoteLeader{
		Term: p.peers.GetTerm(),
		LeaderAddr: leaderAddr,
		Agreed: true,
	})
	if err != nil{
		log.Println("FAILED TO ENCODE LEADER REPLY")
	}	
	p.sendRequest(leaderAddr, rpcPayload, network.RAFT_VOTE_LEADER )
}

func (p *PoC) sendReplyCandidateVote() error{
	leaderAddr, _ := p.peers.GetLeaderAddr()
	rpcPayload, err := encodePayload(VoteCandidate{
		Term: p.peers.self.GetTerm(),
		LeaderAddr: leaderAddr,
		CandidateAddr: p.peers.GetAddr(),	
		Agreed: true,
	})
	if err != nil{
		log.Println("FAILED TO ENCODE LEADER REPLY")
	}	
	p.sendRequest(leaderAddr, rpcPayload, network.RAFT_VOTE_CANDIDATE)
	return nil
}

func(p *PoC) sendRequestCandidateVote(candidate string) error{
	rpcPayload, err := encodePayload(VoteCandidate{
		Term: p.peers.GetTerm(),
		LeaderAddr: p.peers.GetAddr(),
		CandidateAddr: candidate,
		Agreed: false,
	})
	if err != nil {
		return err
	}
	// NOTE: Assign port on init
	p.sendRequest(candidate, rpcPayload, network.RAFT_VOTE_CANDIDATE)
	return nil
}


func(p *PoC) sendRequest(targetAddr string, payload []byte, dt network.DataType) {
	p.net.SendRequest(&net.UDPAddr{IP: net.ParseIP(targetAddr), Port: TESTING_PORT}, payload, dt)
}


func (p *PoC) StartPoc(){
	go p.net.Listen()
	go p.startModifiedRAFT()
	t := 0
	for{
		select{
		case incPacket := <-p.netChan:
			go p.handleRequest(incPacket)
		case <-time.After(time.Second):
			t+=1	
			fmt.Println(t,"(S) PASSED IN MAIN POC")
		}

	}
}

// TODO: Replace each and single log call
func (p *PoC)roleLog(msg string){
	log.Printf("[%s/%d] %s", p.peers.GetRoleString(), p.peers.GetTerm(), msg)
}

// TODO: Remove
func (p *PoC) TestingRegisterPeer(ip string, hostname string){
	p.registerPeer(ip, hostname)
}

