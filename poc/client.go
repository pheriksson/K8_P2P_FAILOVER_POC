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
	HEARTBEAT_FREQ = 10
)

// Need routing table for other members.
//

type PoC struct{
	kube		*kube.KubeClient	
	kubeChan	chan string
	net		*network.Network
	netChan		<-chan network.Packet
	cli		string
	cliChan		<-chan	string 
	peers		*PeerRouting
	
}

func InitPoC(ip string, port int) *PoC{
	nch := make(chan network.Packet)
	n := network.InitNetwork(ip, port, nch)
	p := PoC{kubeChan: make(chan string),
		net: n,
		netChan: nch,
		peers: InitPeerRouting(),
	}
	return &p
}

// Add to routing table. Set in X check to send ping. 
func (p *PoC) registerPeer(addr string, contact string){
	// Basically register peer and start a ttl on the target.
	err := p.peers.AddPeer(addr,contact, HEARTBEAT_FREQ)
	if err != nil{
		log.Printf("%s ALREADY REGISTERED AS PEER", addr)
		return
	}
	// Init heartbeat to caller
	go p.startHeartbeat(addr, time.Duration(time.Second*HEARTBEAT_FREQ))	
}


// Recursivly call continiously for each member until deregistered.
// t+n, after n, if current time == ttl(=t+n)-> need to send heartbeat.
// else, just reschedule new heartbeat timer for. 
// t0+n (heartbeat), t1+n (new time), t1>t0, t1-t0<n -> current-t1+n = time left before heartbeat required.
// Should work
func (p *PoC) startHeartbeat(addr string, heartbeat time.Duration){
	fmt.Println("Setting new heartbeat in: ", heartbeat)
	fmt.Println(heartbeat.Round(time.Second))
	go func(){
		select{
		case <-time.After(heartbeat*time.Second):
		fmt.Println("Time expired.")	
		status, nextHeartbeat, err := p.peers.GetHeartbeatRequired(addr)
		if err != nil {
			log.Printf("STOPPING HEARTBEAT TO [%s]. REASON: [%s]", addr, err)
			return
		}
		if status {
			fmt.Println("<------- SEND HEARTBEAT TO CALLER ------->")
		}
		fmt.Println("Starting timer for next heartbeat in :", nextHeartbeat.Seconds())
		p.startHeartbeat(addr, nextHeartbeat)
		}
	}()

}


func (p *PoC) handleRequest(packet network.Packet){
	// Currently treaing any incoming rpc as a heartbeat.
	fmt.Println("Recieved packet:", packet)
}


func (p *PoC) StartPoc(){
	go p.net.Listen()
	p.registerPeer("127.0.0.1", "SVEN")
	t := 0
	l := true
	for{
		select{
		case incPacket := <-p.netChan:
			go p.handleRequest(incPacket)
		case <-time.After(time.Second):
			t+=1	
			fmt.Println("",t,"(S) PASSED IN MAIN POC")
			if t >14 && l{
				l=false
				fmt.Println("SIMULATING AN INCOMING REQUEST")
				p.peers.UpdateTTL("127.0.0.1")	
			}
		}

	}
}

func (p *PoC) TestSendLocalRequest(port int, msg string){
	p.net.SendRequest(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port} , []byte(msg))
}


