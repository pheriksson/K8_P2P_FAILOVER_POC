package raft

import ( "fmt"
	"log"
	"net"
	"reflect"
	"sync"
	"time"
	"math/rand"

	"github.com/pheriksson/K8_P2P_FAILOVER_POC/kube"
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/network"
)


const(
	SEC_TIME_HEARTBEAT_FREQ=5
	SEC_TIME_TIMEOUT_COMMIT_NOT_LEADER=3
	SEC_TIME_TIMEOUT_FETCH_OLD_COMMIT_WHILE_CONFIRM=2
	SEC_TIME_SEND_LEADER_ELECTION=5
	MILLI_SEC_TIME_TIMEOUT_FAILED_CANDIDACY=5000
	SEC_TIME_TIMEOUT_LEADER_HB=20
)

type RaftStatus int

const (
	LEADER RaftStatus = iota
	CANDIDATE
	MEMBER
)

func (r *RAFT) toString() string{
	switch r.getRole(){
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


type RAFT struct{

	role		RaftStatus
	roleMut		sync.Mutex

	newTerm         chan bool
	term		int
	termMut		sync.Mutex
	peers		*PeerRouting 
	quorum		int


	// network 
	net		*network.Network
	incRpc		<-chan network.RPC

	supportingCandidate string
	supportingcandidateMut sync.Mutex

	newLeaderCh chan string

	// True state of logs.
	commitLog	[]kube.KubeCmd
	commitMut       sync.Mutex

	// Channels for objects not confirmed.
	appendEntryCli <-chan kube.KubeCmd
	appendEntryPeer <-chan kube.KubeCmd

	raftPort int

	// On addition to commit log also write to local kube cluster.
	toCluster chan<- kube.KubeCmd 
}

func InitRaft(ip string, port int, cliCh <-chan kube.KubeCmd) (*RAFT, <-chan kube.KubeCmd){
	nch := make(chan network.RPC)
	n := network.InitNetwork(ip, port, nch)
	kubeCmd := make(chan kube.KubeCmd)
	rand.Seed(time.Now().UnixNano())
	r := &RAFT{
		net: n,
		incRpc: nch,
		peers: InitPeerRouting(ip),
		role: MEMBER,
		roleMut: sync.Mutex{},
		term: 0,
		commitLog: []kube.KubeCmd{},
		commitMut: sync.Mutex{},
		appendEntryCli: cliCh,
		appendEntryPeer: make(chan kube.KubeCmd),
		toCluster: kubeCmd,
		supportingCandidate: "",
		supportingcandidateMut: sync.Mutex{},
		newLeaderCh: make(chan string),
		raftPort: port,
	} 
	return r, kubeCmd
}

func (r *RAFT) getRole() RaftStatus{
	r.roleMut.Lock()
	defer r.roleMut.Unlock()
	return r.role
}

func (r *RAFT) isSupportingCandidate() bool{
	r.supportingcandidateMut.Lock()
	defer r.supportingcandidateMut.Unlock()
	return r.supportingCandidate != ""
}


func (r *RAFT) changeRole(newRole RaftStatus){
	r.roleMut.Lock()
	defer r.roleMut.Unlock()
	r.role = newRole
}

func (r *RAFT) isRole(compareRole RaftStatus) bool{
	r.roleMut.Lock()
	defer r.roleMut.Unlock()
	return r.role == compareRole
}

func (r *RAFT) updateRole(newRole RaftStatus){
	r.roleMut.Lock()
	defer r.roleMut.Unlock()
	r.role = newRole
}

func (r *RAFT) getTerm() int{
	r.termMut.Lock()
	defer r.termMut.Unlock()
	return r.term
}

func (r *RAFT) updateTerm(newTerm int){
	r.termMut.Lock()
	defer r.termMut.Unlock()
	r.term = newTerm
}

func (r *RAFT) addCommit(commitConfirmed kube.KubeCmd){
	r.commitMut.Lock()
	defer r.commitMut.Unlock()
	if reflect.DeepEqual(r.commitLog, []kube.KubeCmd{}){
		r.commitLog = []kube.KubeCmd{commitConfirmed}
	}else{
		r.commitLog = append(r.commitLog, commitConfirmed)
	}
	go func(){
		// Write commit to kube.
		r.toCluster<-commitConfirmed
	}()
}

// Used to catch upp peer.
func (r *RAFT) getSliceLog(lastEntry int) []kube.KubeCmd{
	r.commitMut.Lock()
	defer r.commitMut.Unlock()
	return r.commitLog[lastEntry:]
}

func (r *RAFT) printLog(items int) {
	if items == 0 {
		log.Println("EMPTY COMMIT LOG")
		return 
	}
	for n := 0; n < len(r.commitLog)+1; n++{
		item := r.getLogItem(n)
		log.Printf("AT INDEX: [%d/%d] %s", n, items, item.ToString())
	}


}
func (r *RAFT) getLogItem(index int) kube.KubeCmd{
	r.commitMut.Lock()
	defer r.commitMut.Unlock()
	if index == 0 {
		return kube.KubeCmd{}
	}
	return r.commitLog[index-1]
}

func (r *RAFT) getLengthLog() int{
	r.commitMut.Lock()
	r.commitMut.Unlock()
	count := 0
	for _, entry := range r.commitLog{
		if !reflect.DeepEqual(entry, kube.KubeCmd{}){
			count++
		}
	}
	return count 
}

func (r *RAFT) startRAFT(){
	r.quorum = r.peers.LockRoutingTableEntries()
	for{
		switch r.getRole(){
		case LEADER:
			r.startLeader()
		case CANDIDATE:
			newTerm := r.getTerm()+1
			r.updateTerm(newTerm)
			electionResult := r.requestLeaderVotes(newTerm)
			select{
				case success := <-electionResult:
				if success {
					r.roleLog("RECIEVED MAJORITY - PROMOTED TO LEADER")
					r.changeRole(LEADER)
				}
				case <-r.newLeaderCh:
					r.changeRole(MEMBER)
					time.Sleep(time.Duration(rand.Intn(MILLI_SEC_TIME_TIMEOUT_FAILED_CANDIDACY))*time.Millisecond)
				case <- r.newTerm:
					r.peers.MakeLeader("")
					r.changeRole(MEMBER)
					time.Sleep(time.Duration(rand.Intn(MILLI_SEC_TIME_TIMEOUT_FAILED_CANDIDACY))*time.Millisecond)
			}
		case MEMBER:
			r.watchLeader()
			r.changeRole(CANDIDATE)
		}
	}
}

func (r *RAFT) requestLeaderVotes(newTerm int) (chan bool){
	r.roleLog(fmt.Sprintf("STARTING LEADER VOTE"))
	aggChan := make(chan network.Packet)
	members := r.peers.GetPeersAddress()
	selfAddr := r.peers.GetAddr()
	logIndex := r.getLengthLog()

	for _, peer := range members{
		go func(self string, pr string, logIndex int, term int){
			if pr != "" {
			err, respChannel := r.sendRequestLeaderVotes(self, pr, term, logIndex)
			if err != nil{
				r.roleLog(fmt.Sprintf("FAILED TO SEND LEADER VOTE REQUEST TO [%s]", pr))
			        return
			}
        	        r.roleLog(fmt.Sprintf("SUCCESFULLY SENT LEADER VOTE REQUEST TO [%s] - AWAITING RESPONSE ", pr))
			peerVote := <-respChannel
			aggChan <- peerVote
			}
		}(selfAddr, peer, logIndex, newTerm)
	}
	recievedQourum := make(chan bool)
	go func(done chan bool, aggChan chan network.Packet){
		numVotes := 0
		for{
			select{
			case <-aggChan:
				numVotes+= 1
				r.roleLog(fmt.Sprintf("RECIEVED ANOTHER VOTE FOR LEADERSHIP, REQUIRED: [%d/%d]", numVotes, r.quorum))
				if (numVotes >= r.quorum){
					done<- true
					return 
				}
			case <-time.After(time.Duration(time.Second*SEC_TIME_SEND_LEADER_ELECTION)):
			        r.roleLog("TIMEOUT LEADER ELECTION")
				done<-false
				return
			}
		}
	}(recievedQourum, aggChan)
	return recievedQourum
}

func (r *RAFT) sendRequestCommitObj(self string, peer string, term int, logIndex int, obj kube.KubeCmd) (error, <-chan network.Packet) {
	rpcPayload, err := encodePayload(AppendEntry{
		Term: term,
		LeaderAddr: self,
		LogIndex: logIndex,
		NewObject: obj,
		Confirmed: false,
	})
	if err != nil{
		return fmt.Errorf("FAILED TO ENCODE APPEND REQUEST"), make(chan network.Packet)
	}
	return r.sendRequestAwaitResponse(peer, rpcPayload, network.APPEND_ENTRY)
}

func (r *RAFT) sendRequestLeaderVotes(self string, peer string, term int, logIndex int) (error, <-chan network.Packet) {
	rpcPayload, err := encodePayload(RequestVote{
		Term: term,
		CandidateAddr: self,
		LogIndex: logIndex,
		Agreed: false,
	})
	if err != nil{
		return fmt.Errorf("FAILED TO ENCODE LEADER REQUEST"), make(chan network.Packet)
	}
	return r.sendRequestAwaitResponse(peer, rpcPayload, network.REQUEST_VOTE)
}

// Send request and recieve response in channel.
func (r *RAFT) sendRequestAwaitResponse(targetAddr string, payload []byte, dt network.DataType) (error, chan network.Packet){
	return r.net.SendRequestAwaitResponse(&net.TCPAddr{IP: net.ParseIP(targetAddr), Port: r.raftPort}, payload, dt)
}

// Send request without response channel.
func (r *RAFT) sendRequestNoResponse(targetAddr string, payload []byte, dt network.DataType) (error){
	return r.net.SendRequest(&net.TCPAddr{IP: net.ParseIP(targetAddr), Port: r.raftPort}, payload, dt)
} 

func (r *RAFT) watchLeader() (error){
	leaderTTL, err := r.peers.GetLeaderTimeout()
	if err != nil{
		r.roleLog("NO VALID LEADER ELECTED.")
		return fmt.Errorf("NO VALID LEADER ELECTED IN TERM: %d.\nREASON: %s", r.peers.GetTerm(), err)
	}
	r.roleLog(fmt.Sprintf("START TO AWAIT LEADER TIMEOUT, TIME REMAINING: %d(s).",int(leaderTTL.Seconds())))
	r.awaitLeaderTTL(leaderTTL)				
	return nil
}


// Will return if no heartbeat occurs within leader TTL OR member promotion. 
func (r *RAFT) awaitLeaderTTL(heartbeatTimeout time.Duration){
	select{
		case <-time.After(heartbeatTimeout):
			timeout, nextHeartbeatTimeout, err := r.peers.CheckLeaderTimeout()
			if err != nil || timeout{
				r.roleLog("LEADER TIMEOUT")	
				return 
			}
	                r.roleLog(fmt.Sprintf("HB FROM LEADER EXPECTED IN: %s(s)",nextHeartbeatTimeout.String()))
			r.awaitLeaderTTL(nextHeartbeatTimeout)
		case <-r.newTerm:
			// Need to consume channel for possible candidate and leader states.
			r.roleLog("RECIEVED NEW TERM")
	}
}


// NOTE: Handling appending request in handleRPC loop (if peer wants to add to commit.)
// For appendEntryCli -> simply need to run commitObject and there we check if leader then simply get quorum or else send to leader for quorum.
func (r *RAFT) startLeader(){
	for{
		select{
		case <-time.After(time.Second*SEC_TIME_HEARTBEAT_FREQ):
			if !r.isRole(LEADER){return}
			r.sendLogUpdates(r.getTerm(), r.getLengthLog())
		case <-r.newTerm:
			r.changeRole(MEMBER)
		        return
		}
	}
}

// Used by all members.  
func (r *RAFT) commitObject(obj kube.KubeCmd) bool{
	if r.isRole(LEADER){
		// Simply request quorum and update confirmation.
		accepted := r.requestAddObject(obj)
		quorum := <-accepted
		if quorum {
			r.roleLog(fmt.Sprintf("RECIEVED QUORUM ON COMMIT OBJECT [%s]", obj.ToString()))
			r.addCommit(obj)
			return true
			// Add entry to commit log and announce it to peers.
		}
		return false
	}else if r.isRole(CANDIDATE){
		r.roleLog("AS CANDIDATE CANNOT CONFIRM OR DENY REQUEST FOR NEW REQUEST")
		return false
	}else{
		// as MEMBER
		r.roleLog("TRYING TO COMMIT NEW OBJECT")
		_, leaderError := r.peers.GetLeaderTimeout()
		if leaderError != nil {
			// No valid leader exists.
			return false
		}
		// Will be blocked if self log index is lower than current.
		// i.e. need to fetch before ocmmiting.
		ladr, _ := r.peers.GetLeaderAddr()
		err, resp := r.sendLeaderWrite(r.getTerm(), r.getLengthLog(), ladr, obj)
		if err != nil{
			r.roleLog(fmt.Sprintf("FAILED TO SEND COMMIT REQ TO LEADER, REASON: [%s]",err.Error()))
			return false
		}
		select{
			case <-time.After(time.Second*SEC_TIME_TIMEOUT_COMMIT_NOT_LEADER):
				r.roleLog("FAILED TO COMMIT REQUEST TO LEADER")
				return false
			case packet := <-resp:
				rpc, _ := decodePayload[AppendEntryResponse](packet.Data)
				if !rpc.Confirmed {
			                r.roleLog("FAILED TO COMMIT DATA - LEADER DENIED REQUEST")
					return false
				}
		                r.roleLog("SUCCESSFULLY RECIEVED CONF OF NEW OBJECT FROM LEADER")
				r.addCommit(obj)
				return true
		}
	}
}

func (r *RAFT) sendLeaderWrite(term int, logIndex int, ladr string, newEntry kube.KubeCmd) (error, chan network.Packet){
	rpcPayload, err := encodePayload(AppendEntry{
		Term: term,
		LeaderAddr: ladr,
		LogIndex: logIndex,
		NewObject: newEntry,
		Confirmed: false,
	})
	if err != nil{
		return fmt.Errorf("FAILED TO SEND WRITE TO LEADER"), make(chan network.Packet)
	}
	return r.sendRequestAwaitResponse(ladr, rpcPayload, network.APPEND_ENTRY)
}

// Will request quorum from peers to add object entry.
func (r *RAFT) requestAddObject(obj kube.KubeCmd) (chan bool){
	numVotes := 0
	aggChan := make(chan network.Packet)
	members := r.peers.GetPeersAddress()
	selfAddr := r.peers.GetAddr()
	logIndex := r.getLengthLog()
	newTerm := r.getTerm()
	for _, peer := range members{
		go func(self string, pr string, logIndex int, term int, obj kube.KubeCmd){
			if pr != "" {
			err, respChannel := r.sendRequestCommitObj(self, pr, term, logIndex, obj)
			if err != nil{
        	                r.roleLog(fmt.Sprintf("FAILED TO SEND COMMIT NEW OBJ REQUEST TO [%s] ",pr))
			        return
			}
        	        r.roleLog(fmt.Sprintf("SUCCESFULLY SENT COMMIT NEW OBJ REQUEST TO [%s] ",pr))
			peerVote := <-respChannel
			aggChan <- peerVote
			}
		}(selfAddr, peer, logIndex, newTerm, obj)
	}
	recievedQourum := make(chan bool)
	go func(done chan bool){
		for{
			select{
			case pack := <-aggChan:
				reply, _ := decodePayload[AppendEntryResponse](pack.Data)
				if reply.Confirmed {
					numVotes+= 1
					r.roleLog(fmt.Sprintf("RECIEVED ANOTHER VOTE FOR COMMITING OBJECT, REQUIRED: [%d/%d]", numVotes,r.quorum))
				}
				if (numVotes >= r.quorum){
					done<-true
					return
				}
			case <-time.After(time.Duration(time.Second*SEC_TIME_SEND_LEADER_ELECTION)):
				r.roleLog(fmt.Sprintf("TIMEOUT LEADER ELECTION, RECIEVED: [%d/%d]", numVotes,r.quorum))
				done<-false
				return
			}
		}
	}(recievedQourum)
	return recievedQourum
}



func (r *RAFT) sendLogUpdates(term int, logIndex int){
	rpcPayload, err := encodePayload(AppendEntry{
		Term: term,
		LeaderAddr: r.peers.GetAddr(),
		LogIndex: logIndex,
	})
	if err != nil{
		log.Println("FAILED TO SEND PING UPDATE")
	}
	for _, peer := range r.peers.GetPeersAddress() {
		if peer == "" {continue}
		//r.roleLog("SENDING PING TO:"+peer+"")
		r.sendRequestNoResponse(peer, rpcPayload, network.APPEND_ENTRY)
	} 
}

func (r *RAFT) handleRpc(rpc network.RPC){
	rp := rpc.GetPacket()
	callerAddrs := rp.Caller.IP.String()
	if !r.peers.IsPeer(callerAddrs){
		r.roleLog(fmt.Sprintf("UNKNOWN PEER [%s]", callerAddrs))
		return
	}
	err := r.peers.UpdateTTL(callerAddrs)
	if err != nil{
		r.roleLog(fmt.Sprintf("FAILED TO UDPATE TTL FOR [%s]",callerAddrs))
	}
	switch rp.Type{
	case  network.REQUEST_VOTE:
		data, _ := decodePayload[RequestVote](rp.Data)
		if data.Term > r.getTerm() {
			r.updateTerm(data.Term)
			r.changeRole(MEMBER)
			if data.LogIndex < r.getLengthLog() {
			r.roleLog("RECIEVED NEW TERM")
				r.peers.WipeLeader()
				close(rpc.Done)
			}else if  data.LogIndex >= r.getLengthLog() {
				r.roleLog("RECIEVED NEW LEADER")
				r.peers.MakeLeader(data.CandidateAddr)
				r.sendReplyLeaderVote(rpc.Done, true)
			}
			r.newTerm <- true
		}else if data.Term == r.getTerm(){
			// Same term - if caller has higher commitLog simple back off and await.
			if data.LogIndex > r.getLengthLog() {
				r.peers.MakeLeader(data.CandidateAddr)
				if r.isRole(CANDIDATE){
				    r.newLeaderCh <- data.CandidateAddr
				}
				r.sendReplyLeaderVote(rpc.Done, true)  
				return
			}
		}else{
			r.roleLog("RECIEVED STALE TERM")
		}
	case network.APPEND_ENTRY:
		data, _ := decodePayload[AppendEntry](rp.Data)
		objectNil := reflect.DeepEqual(data.NewObject, kube.KubeCmd{})
		if data.Term >= r.getTerm() && data.LogIndex > r.getLengthLog() && r.isRole(CANDIDATE){
			r.updateTerm(data.Term)
			r.peers.MakeLeader(data.LeaderAddr)
			r.newLeaderCh <- data.LeaderAddr
		}else if data.Term > r.getTerm(){
			r.updateTerm(data.Term)
			r.newTerm <- true
		}else{
			if r.isRole(LEADER) && !objectNil{
				r.roleLog("RECIEVED REQUEST TO COMMIT DATA")
				if data.LogIndex == r.getLengthLog() {
					added := r.commitObject(data.NewObject)
					if added {
						r.roleLog("DATA COMMITTED")
						r.sendReplyObjectCommited(rpc.Done, added)
					}
				}else{
					r.roleLog("PEER HAS INVALID COMMIT LOG")
					r.sendReplyObjectCommited(rpc.Done, false)
				}
			}else{
				if r.getLengthLog() == data.LogIndex && objectNil{ 
					//r.roleLog("RECIEVED LEADER PING")
					return 
				}else if r.getLengthLog() == data.LogIndex{
					r.roleLog(fmt.Sprintf("ACCEPTING VOTE FOR NEW OBJECT, CURR INDEX [%d]", data.LogIndex))
					r.sendReplyObjectCommited(rpc.Done, true)
					return
				}else{
					latestLogsToFetch := data.LogIndex - r.getLengthLog()
					ladr, _ := r.peers.GetLeaderAddr()
					term := r.getTerm()
					for n := 0; n < latestLogsToFetch; n++{
						fetchIndex := r.getLengthLog()+1
						r.roleLog(fmt.Sprintf("TRYING TO FETCH INDEX [%d]", fetchIndex))
						// Fetching and handling rpc response here as we're able to return a confirmed request
						// without missing a commit round. 
						err, resp := r.sendFetchObjectAtIndex(term, fetchIndex, ladr)
						if err != nil{
							r.roleLog(fmt.Sprintf("FAILED TO FETCH INDEX [%d]", fetchIndex))
							return
						}
						select{
							case <-time.After(time.Second*SEC_TIME_TIMEOUT_FETCH_OLD_COMMIT_WHILE_CONFIRM):
								r.roleLog(fmt.Sprintf("FAILED TO FETCH INDEX [%d]", fetchIndex))
								return
							case packet:=<-resp:
								data, _ := decodePayload[FetchMissingEntries](packet.Data)
								if reflect.DeepEqual(data.NewObject, kube.KubeCmd{}) || data.LogIndex != fetchIndex{
									r.roleLog(fmt.Sprintf("FAILED TO FETCH INDEX [%d]", fetchIndex))
									return
								}
								log.Printf("ADDING NEW OBJECT: %v", data.NewObject)
								r.addCommit(data.NewObject)
						}
					}
					go func(){ 
						// Try to send if connection is still live of accepting the new commit from leader.
					        r.roleLog(fmt.Sprintf("ACCEPTING VOTE FOR NEW OBJECT, CURR INDEX [%d]", data.LogIndex))
						r.sendReplyObjectCommited(rpc.Done, true)
					}()
				}

			}
		}
	case network.FETCH_MISSING_ENTRIES:
		data, _ := decodePayload[FetchMissingEntries](rp.Data)
		if data.Term > r.getTerm() {
			r.roleLog("RECIEVED FETCH ON NEWER TERM - OLD MEMBER THINKS SELF IS LEADER")
			return
		}
	        if r.isRole(LEADER) && data.Term <= r.getTerm() && data.LogIndex <= r.getLengthLog(){
			requestedObject := r.getLogItem(data.LogIndex)
			r.respondFetchMissingEntries(rpc.Done, rpc.P.Caller.String(),requestedObject, data.LogIndex, data.LeaderAddr)
			return
		}
log.Printf("UNKNOWN EVEN IN FETCH MISSING ENTRIES #%v", data)
	}
}
// TODO: Look over RPC/Packet structure (do i even need term in fethcingmissingentries rpc & leaderaddr??.).
func (r *RAFT) respondFetchMissingEntries(replyChan chan<-[]byte, caller string, obj kube.KubeCmd, index int, laddr string){
	rpcPayload, err := encodePayload(FetchMissingEntries{
		LogIndex: index,
		NewObject: obj,
	})
	if err != nil{
		log.Println("FAILED TO ENCRYPT RETURN MESSAGE OF MISSING LOGS.")
	}
	r.roleLog(fmt.Sprintf("SENDING COMMIT [%d] TO [%s]", index, caller))
	r.sendEncodedReply(laddr, rpcPayload, network.FETCH_MISSING_ENTRIES, replyChan)
}

// TODO: Go over all the different reqest response calls. as this is just silly.
func (r *RAFT) sendFetchObjectAtIndex(term int, index int, laddr string) (error, chan network.Packet) {
	rpcPayload, err := encodePayload(FetchMissingEntries{
		Term: term,
		LeaderAddr: laddr,
		LogIndex: index,
	})
	if err != nil{
		return fmt.Errorf("FAILED TO SEND WRITE TO LEADER"), make(chan network.Packet)
	}
	r.roleLog(fmt.Sprintf("SENDING FETCH ENTRY FOR INDEX: [%d] FROM LEADER: [%s]", index, laddr))
	return r.sendRequestAwaitResponse(laddr, rpcPayload, network.FETCH_MISSING_ENTRIES)
}

func (r *RAFT) supportCandidate(addr string){
	r.supportingcandidateMut.Lock()
	defer r.supportingcandidateMut.Unlock()
	r.supportingCandidate = addr 
}

// Returns true if support was being held for cand.
func (r *RAFT) stopSupport() bool{
	r.supportingcandidateMut.Lock()
	defer r.supportingcandidateMut.Unlock()
	supported := (r.supportingCandidate != "")
	r.supportingCandidate = "" 
	return supported

}

func (r *RAFT) sendReplyObjectCommited(replyChan chan<-[]byte, status bool){
	leaderAddr, err := r.peers.GetLeaderAddr()
	if err != nil{
		r.roleLog("NO LEADER TO BE FOUND, REASON: "+err.Error())
	}
	rpcPayload, err := encodePayload(AppendEntryResponse{
		Term: r.getTerm(),
		LeaderAddr: leaderAddr,
		LogIndex: r.getLengthLog(),
		Confirmed: status,
	})
	if err != nil{
		log.Println("FAILED TO ENCODE LEADER REPLY")
	}	
	r.sendEncodedReply(leaderAddr, rpcPayload, network.REQUEST_VOTE, replyChan)

}

func (r *RAFT) sendReplyLeaderVote(replyChan chan<- []byte, status bool){
	// Will only be sent if new leader is in greater term && has higher or same commitLog.
	leaderAddr, err := r.peers.GetLeaderAddr() 
	if err != nil{
		r.roleLog("NO LEADER TO BE FOUND, REASON: "+err.Error())
	}
	rpcPayload, err := encodePayload(RequestVote{
		Term: r.getTerm(),
		CandidateAddr: leaderAddr,
		Agreed: true,
	})
	if err != nil{
		log.Println("FAILED TO ENCODE LEADER REPLY")
	}	
	r.roleLog(fmt.Sprintf("SENDING REPLY LEADER VOTE, CURRENT LEADER: [%s]", leaderAddr))
	r.sendEncodedReply(leaderAddr, rpcPayload, network.REQUEST_VOTE, replyChan)
}

func (r *RAFT)sendEncodedReply(targetAddr string, payload []byte, dt network.DataType, ch chan<- []byte) {
	packet := network.NewPacket(&net.TCPAddr{IP: net.ParseIP(targetAddr), Port:r.raftPort}, payload, dt) 
	encPacket, err := network.Encode(*packet) 
	if err != nil{
		log.Printf("FAILED TO ENCODE PACKET ADDRESSED TO %s", targetAddr)
	}
        go func(){
		ch <- encPacket
	}()
}

func (r *RAFT) Start(peers []string){
	for _, peer := range peers{
		r.peers.AddPeer(peer, "")
	}
	go r.net.Listen()
	go func(){time.Sleep(time.Second); r.startRAFT()}()
	for{
		select{
		case incRPC := <-r.incRpc:
			go r.handleRpc(incRPC)
		case incCommit := <-r.appendEntryCli:
			go func(){
			        r.roleLog(fmt.Sprintf("GOT CALL TO APPEND OBJECT: %s", incCommit.ToString()))
				status := r.commitObject(incCommit)
				if status{
					log.Println("SUCESSFULLY CREATED OBJECT:", incCommit.ToString())
				}else{
					log.Println("FAILED TO CREATE OBJECT:", incCommit.ToString())
				}
			
			}()
		//case <-time.After(time.Second*5):
		//	r.stats()
		}
	}
}

func (r *RAFT) stats(){
	log.Println("RAFT STATUS UPDATE:")
	r.roleLog(fmt.Sprintf("COMMIT INDEX: %d", r.getLengthLog()))
	r.printLog(r.getLengthLog())
}

func (r *RAFT) roleLog(msg string){
	log.Printf("[%s:%d] %s", r.toString(), r.getTerm(), msg)
}

