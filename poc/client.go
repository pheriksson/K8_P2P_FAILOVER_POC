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
)

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

// Register peer and start heartbeat call on target. 
func (p *PoC) registerPeer(addr string, contact string){
	err := p.peers.AddPeer(addr,contact, HEARTBEAT_FREQ)
	if err != nil{
		log.Printf("%s ALREADY REGISTERED AS PEER", addr)
		return
	}
	go p.startHeartbeat(addr, time.Duration(time.Second*HEARTBEAT_FREQ))	
}

func (p *PoC) startHeartbeat(addr string, heartbeat time.Duration){
	go func(){
		select{
		case <-time.After(heartbeat):
		status, nextHeartbeat, err := p.peers.GetHeartbeatRequired(addr)
		if err != nil {
			log.Printf("STOPPING HEARTBEAT TO [%s]. REASON: [%s]", addr, err)
			return
		}
		if status {
			// INSERT: PING ADDR
			func(){
				for n:= 0; n<HEARTBEAT_TIMEOUT_VALID_MULTIPLIER; n++{
					select{
					case <-time.After(time.Duration(HEARTBEAT_FREQ*time.Second)):
						valid, err := p.peers.ValidTTL(addr)
						if err != nil{
							log.Panic("UNEXPECTED ENTRY REMOVED. ERR:",err)	
						}
						if valid {return}	
					}
				}
				p.peers.DeactivatePeer(addr)
			}()
			nextHeartbeat = time.Duration(HEARTBEAT_FREQ*time.Second)
		}
		p.startHeartbeat(addr, nextHeartbeat)
		}
	}()
}

func (p *PoC) handleRequest(packet network.Packet){
	// Currently treaing any incoming rpc as a heartbeat.
	// Call to updateTTL for user at packet and set their status to active.

	fmt.Println("Recieved packet:", packet)
	p.peers.UpdateTTL(packet.Caller.IP.String())	
}


func (p *PoC) StartPoc(){
	go p.net.Listen()
	p.registerPeer("127.0.0.1", "SVEN")
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


