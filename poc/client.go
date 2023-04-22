package poc

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/pheriksson/K8_P2P_FAILOVER_POC/kube"
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/network"
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/proxy"
)

const (
	// Time intervall for leader to send heartbeats to peers.
	HEARTBEAT_FREQ = 5
	// Multiplier for how many heartbeat intervalls members are supposed to wait for leader updates.
	HEARTBEAT_TIMEOUT_VALID_MULTIPLIER = 5 
	// Time in secconds of the leader timeout range for members to await health pings.
	TTL_TIMEOUT_SECONDS = HEARTBEAT_FREQ*HEARTBEAT_TIMEOUT_VALID_MULTIPLIER
	// Time for member/candidate running for leader should wait for confirmed votes by other peers. 
	TIME_SEC_SEND_LEADER_ELECTION = 5
	// Time intervall for leader to send volumes to candidate.
	TIME_SEC_SEND_VOLUME_CANDIDATE = 20
	MODIFIED_RAFT_PORT = 9999
	MODIFIED_RAFT_PORT_FILE_TRANSFER = 9998
	// Time for leader to await response from candidate of confirming a candidency under self as leader.
	TIME_SEC_AWAIT_CANDIDATE_RESPONSE = 10
	// Time for leader to await resposne from candidate confirming volume transfer
	TIME_SEC_AWAIT_CANDIDATE_VOLUME_CONFIRMATION = 30
	// Random timeout range for members awaiting other member running for leader. 
	RANDOM_TIMEOUT_RANGE_MEMBER_MS = 10000
	// Static string used as placeholder for packets (verbose msg)
	MISSING_CANDIDATE_STRING = "TBD"
)


type RaftStatus int

const (
	LEADER RaftStatus = iota
	CANDIDATE
	MEMBER
)


type PoC struct{
	kube		*kube.KubeClient	
	fromKube	<-chan kube.KubeMsg
	toKube		chan<- kube.KubeMsg
	kubeVolumes     chan []string
	net		*network.Network
	netChan		<-chan network.RPC
	proxy           *proxy.Proxy
	proxyLeader	chan<- string
	cli		string
	cliChan		<-chan	string 
	peers		*PeerRouting
	role		RaftStatus	
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
	r		int 
}


func InitPoC(ip string, port int, kubeConf string ) *PoC{
	nch := make(chan network.RPC)
	n := network.InitNetwork(ip, port, MODIFIED_RAFT_PORT_FILE_TRANSFER, nch)

	proxyLeaderChan := make(chan string)


	kubPorts := make(chan []int)
	kubNodes := make(chan []string)

	pr := proxy.InitProxy(proxyLeaderChan , kubNodes, kubPorts, ip)

	toKube := make(chan kube.KubeMsg)
	fromKube := make(chan kube.KubeMsg)

	k := kube.InitKubeClient(kubeConf, kubNodes, kubPorts,  toKube, fromKube)

	p := PoC{
		kube: k,
		fromKube: fromKube,
		toKube: toKube,
		net: n,
		netChan: nch,
		peers: InitPeerRouting(ip),
		proxy: pr,
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
		r: rand.Intn(RANDOM_TIMEOUT_RANGE_MEMBER_MS),
		proxyLeader: proxyLeaderChan,
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
	aggChan := make(chan network.Packet)
	members := p.peers.GetPeersAddress()
	for _, peer := range members{
		go func(pr string, term int){
			if pr != "" {
			err, respChannel := p.sendRequestLeaderVotes(pr, term)
			if err != nil{
        	                p.roleLog("FAILED TO SEND LEADER VOTE REQUEST TO "+pr)
			        return
			}
        	        p.roleLog("SUCCESFULLY SENT LEADER VOTE REQUEST TO "+pr)
			peerVote := <-respChannel
			aggChan <- peerVote
			}
		}(peer, p.peers.GetTerm())
	}
	for{
		select{
		case <-aggChan:
			numVotes+= 1
		        p.roleLog("RECIEVED ANOTHER VOTE, TOTAL:"+strconv.Itoa(numVotes))
			if (numVotes >= p.quorum){
				p.peers.BecomeLeader()
				p.roleLog("NEW ROLE: LEADER")
				p.proxyLeader<-p.peers.GetAddr()
			        // Bootstrap leader pings before initing leader states.
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
		p.roleLog("NEW TERM")
		if p.peers.BecomeMember(p.peers.GetTerm()){
			p.roleLog("DEMOTED TO MEMBER")
		}
		return
	case <- p.startEarlyFailover:
		//
		p.roleLog("<<---- TODO: INSERT EARLY FAILOVER ---->>")
		if (p.peers.GetRole() == CANDIDATE){
			p.awaitKubernetesCluster()
			return
		}
	}	
}

func (p *PoC) awaitKubernetesCluster(){
	p.toKube<-kube.START_CLUSTER
	for{
		select{
		case <-time.After(time.Second):
			log.Println("Awaiting k8 cluster takeover")	
		case msg := <-p.fromKube:
			log.Println("RECIEVED MSG FROM K8 CLUSTER:", msg)
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
		err, candResp := p.sendRequestCandidateVote(peer) 
		if err != nil{
			time.Sleep(time.Second*TIME_SEC_AWAIT_CANDIDATE_RESPONSE)
			continue
		}
		select{
		case <-time.After(time.Second*TIME_SEC_AWAIT_CANDIDATE_RESPONSE):
		        p.roleLog("FAILED TO GET CONFIRMATION FROM: "+peer)	
		case confirmed :=<-candResp:
			data, err := decodePayload[VoteCandidate](confirmed.Data)
			if err != nil {
                	     log.Println("ERROR DECODING FOR ASSIGN CANDIDATE")
			}
			if data.Agreed && peer == data.CandidateAddr{
					p.roleLog("GOT CANDIDATE CONFIRMATION FROM: "+data.CandidateAddr)
					p.peers.MakeCandidate(data.CandidateAddr)
					return true	

			}
		case <-abort:
			p.roleLog("ABORTING CANDIDATE PROMOTION")
			return false
		}
	}
	p.roleLog("CHECKED WITH ALL MEMBERS - RESTARTING CANDIDATE SELECTION")
	return p.assignCandidate(abort)

}

// TODO: Fix better logging.
// TODO: Unhealthy cluster. Notfiy leader of promoting candidate.
func (p *PoC) startHeartbeat(heartbeat time.Duration, stopVolumeRpc chan<- bool, stopHeartbeat <-chan bool){
	members := p.peers.GetPeersAddress()
	for{
		select{
		case <-time.After(heartbeat):
			p.sendLeaderPings(members)
		case <-p.oldTerm:
			p.roleLog(fmt.Sprintf("REGISTERED NEW TERM OF: %d", p.peers.GetTerm()))
			stopVolumeRpc<-true 
			return
		case <-stopHeartbeat:
			p.roleLog("HEARBEAT - RECIEVED STOP FROM LEAVING LEADER STATE")
			return
	}
	}
}
// Need to check for go-routine memory leaks.
func (p *PoC) startVolumeWindowCandidate(stopFromHeartbeat chan bool, stopHeartbeat chan bool){
	p.roleLog("INITIATING PERSISTENT VOLUME WINDOW TRANSFER")
	for{
		select{
		case <-time.After(time.Second*TIME_SEC_SEND_VOLUME_CANDIDATE):
			candAddr, err := p.peers.GetCandidateAddress()
			if err != nil{
				p.roleLog("INVALID STATE: MISSING CANDIDATE")
				stopHeartbeat<-true	
				return
			}
			//TODO: implement kube cntrller 'volumes := p.kube.GetLatestVolumesPaths()' 
			TBD_GET_FUNCION := []string{"3gb_file.txt","9gb_file.txt"}
			abortVolume := make(chan bool, len(TBD_GET_FUNCION))
			volumesSent := p.sendCandidatePrepareVolumeTransfer(candAddr, TBD_GET_FUNCION, abortVolume)
			select{
				case <-time.After(time.Second*TIME_SEC_AWAIT_CANDIDATE_VOLUME_CONFIRMATION):
					for n := 0; n < len(TBD_GET_FUNCION); n++ {abortVolume <- true}	
					stopHeartbeat<- true
					return
				case transferStatus := <-volumesSent:
					if transferStatus {
						p.roleLog("CANDIDATE CONFIRMED SUCCESFULL VOLUME TRANSFER")
					}else{
						p.roleLog("FAILED VOLUME TRANSFER TO CANDIDATE")
					}
			}	
		case <-stopFromHeartbeat:
			// New term recieved.
			return
		}
	}

}

func (p *PoC) handleRequest(rpc network.RPC){
	packet := rpc.GetPacket()
	callerAddrs := packet.Caller.IP.String()
	if !p.peers.IsPeer(callerAddrs){
		p.roleLog("UNKNOWN PEER REQUEST - ["+callerAddrs+"]"+" PACKET TYPE:"+packet.Type.ToString())
		return
	}
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
				// New term, meaning only possible packet is a request to become leader.
				if val.LeaderAddr != p.peers.GetAddr(){
					// Used to handle edge case response from Case (1) when a response is returned to self 
					// TODO Assert that k8 cluster is active
					p.sendReplyLeaderVote(rpc.Done)
				}
				return
			}
			currLeader, _ := p.peers.GetLeaderAddr()
			if val.Term < p.peers.GetTerm() && !val.Agreed && val.LeaderAddr == currLeader{
				p.sendReplyLeaderVote(rpc.Done)
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
				p.roleLog("RECIEVED REQUEST TO BECOME CANDIDATE ON NEWER TERM - ACCEPT AND AWAIT CONF.")
				p.sendReplyCandidateVote(rpc.Done)
				return
			}
			if (val.Term == p.peers.GetTerm()){
				// Same term, leader requesting candidate. 
				lAddr, err := p.peers.GetLeaderAddr()
				if val.LeaderAddr == lAddr && err == nil && !val.Agreed{ 
					// Candidate responding on same term
					p.sendReplyCandidateVote(rpc.Done)
					return
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
			// NOTE: Ordering matters, p.checkTerm needs to be on the LHS of expression.
			if (p.checkTerm(val.Term, val.LeaderAddr) || val.Term == p.peers.GetTerm()){
				if (p.peers.MakeCandidate(val.CandidateAddr)){
					p.roleLog("NEW ADDRESS REGISTERED FOR CANDIDATE: "+val.CandidateAddr)
				}
				if (!val.Stable && p.peers.GetRole() == CANDIDATE){
					p.roleLog("UNSTABLE LEADER - AS CANDIDATE BECOME LEADER EARLY.")
					p.startEarlyFailover <- true
				}
			}
		}()
	}
}

// pre: new term and new leader addr corresponding to term.
// post: If new term - state change for all roles becoming members.
func (p *PoC) checkTerm(newTerm int, leaderAddr string) bool{
	p.oldTermMut.Lock()
	defer p.oldTermMut.Unlock()
	if newTerm > p.peers.GetTerm(){
		p.peers.BecomeMember(newTerm)
		p.peers.MakeLeader(leaderAddr)
		p.proxyLeader <- leaderAddr
		p.roleLog("NEW TERM: "+strconv.Itoa(newTerm)+" AND LEADER: "+ leaderAddr)
		p.oldTerm <- newTerm

		return true 
	}
	return false
}


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

func (p *PoC) sendRequestLeaderVotes(peer string, newTerm int) (error, chan network.Packet){
	rpcPayload, err := encodePayload(VoteLeader{
		Term: newTerm,
		LeaderAddr: p.peers.GetAddr(),
		Agreed: false,
	})
	if err != nil{
		return fmt.Errorf("FAILED TO ENCODE LEADER REQUEST"), make(chan network.Packet)
	}
	p.roleLog("SENDING VOTE REQUEST TO:"+peer)
	return p.sendRequestAwaitResponse(peer, rpcPayload, network.RAFT_VOTE_LEADER)
}

func (p *PoC) sendReplyLeaderVote(replyChannel chan<- []byte) {
	leaderAddr, err := p.peers.GetLeaderAddr() 
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
	p.roleLog("SENDING REPLY LEADER VOTE, CURRENT LEADER:"+leaderAddr)
	sendEncodedReply(leaderAddr, rpcPayload, network.RAFT_VOTE_LEADER, replyChannel)
}
func (p *PoC) sendReplyCandidateVote(replyChannel chan<- []byte) {
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
	p.roleLog("SENDING REPLY ACCEPTING CANDIDATE VOTE")
	sendEncodedReply(leaderAddr, rpcPayload, network.RAFT_VOTE_CANDIDATE, replyChannel)
}

func(p *PoC) sendRequestCandidateVote(candidate string) (error, chan network.Packet){
	rpcPayload, err := encodePayload(VoteCandidate{
		Term: p.peers.GetTerm(),
		LeaderAddr: p.peers.GetAddr(),
		CandidateAddr: candidate,
		Agreed: false,
	})
	if err != nil {
		log.Println("FAILED TO ENCODE PAYLOAD FOR sendRequestCandidateVote")
	}
        return 	p.sendRequestAwaitResponse(candidate, rpcPayload, network.RAFT_VOTE_CANDIDATE)
}
func (p *PoC) sendRequestAwaitResponse(targetAddr string, payload []byte, dt network.DataType) (error, chan network.Packet){
	return p.net.SendRequestAwaitResponse(&net.TCPAddr{IP: net.ParseIP(targetAddr), Port: MODIFIED_RAFT_PORT}, payload, dt)
}

func (p *PoC) sendRequest(targetAddr string, payload []byte, dt network.DataType){
	err := p.net.SendRequest(&net.TCPAddr{IP: net.ParseIP(targetAddr), Port: MODIFIED_RAFT_PORT}, payload, dt)
	if err != nil{
		p.roleLog("FAILED TO SEND REQUEST OF TYPE:["+dt.ToString()+"] TO PEER: ["+targetAddr+"]")
	}
}

func sendEncodedReply(targetAddr string, payload []byte, dt network.DataType, ch chan<- []byte) {
	packet := network.NewPacket(&net.TCPAddr{IP: net.ParseIP(targetAddr), Port:MODIFIED_RAFT_PORT}, payload, dt) 
	encPacket, err := network.Encode(*packet) 
	if err != nil{
		log.Printf("FAILED TO ENCODE PACKET ADDRESSED TO %s", targetAddr)
	}
        go func(){
		ch <- encPacket
	}()

}

// TODO: Look for leaking 
func (p *PoC) sendCandidatePrepareVolumeTransfer(targetIp string,filePaths []string, abort <-chan bool) (chan bool){
	done := make(chan bool)
	// Make space for confirmed transmissions
	confirmedTransmissions := make(chan bool, len(filePaths))
	target := &net.TCPAddr{IP:net.ParseIP(targetIp), Port: MODIFIED_RAFT_PORT_FILE_TRANSFER}
	for _, file := range filePaths{
		go func(target *net.TCPAddr, filePath string, abort <-chan bool){
			p.roleLog("SENDING VOLUME: ["+filePath+"]")
			status := p.net.SendLargeFile(target, filePath)
			select{
		        case <-abort:
				p.roleLog("RECIEVED ABORT SIGNAL IN VOLUME TRANSMISSION FOR VOLUME: ["+filePath+"]")
				return
			case s := <-status:
				if s {
					confirmedTransmissions <- true
					p.roleLog("SUCCESFULLY SENT VOLUME: ["+filePath+"]")
				}else{
					confirmedTransmissions <- false
					p.roleLog("FAILED TO SEND VOLUME: ["+filePath+"]")
				}
			}
		}(target, file, abort)
	}
	go func(n int){
		// Need to pass n succesfull transmissions.
		for i := 0; i < n; i++{
			status:= <-confirmedTransmissions
			if !status{
				p.roleLog("ABORTING TRANSMISSION")
				done<-false
			}
		}
		done<-true

	}(len(filePaths))
	return done
}

// Ok will simply controll start/stop for cluster.
func (p *PoC) StartPoc(){
	log.Panic("WIP - ETCD CLUSTER RESTORATION PLAN TO COMPLICATED")
	go p.net.Listen() // NOTE: UNCOMNET 
	go p.proxy.Start()
	//go p.kube.Start() <- should only start uppon leader election.
	go func(){time.Sleep(time.Second); p.startModifiedRAFT()}()
	t := 0
	for{
		select{
		case incRPC := <-p.netChan:
			go p.handleRequest(incRPC)
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

