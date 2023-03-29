package poc;

import(
	"github.com/pheriksson/K8_P2P_FAILOVER_POC/network"

)


// Need routing table for other members.
//

type PoC struct{
	kubeChan	chan string
	networkChan	chan network.Packet
}


