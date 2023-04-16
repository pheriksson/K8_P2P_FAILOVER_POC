package proxy

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"
)

// TODO
// GetActiveNodes from leader - for loadbalancing.
// GetActiveLeader - If self -> simply process request to nodes // else forward to leader proxy.
// GetActivePorts - basically used to get the accessible ports, start to listen on each of the accessible ports.

// 1. Forward request to leader, if in failover mode, still forward to leader as candidate will assume leader upon active k8 deployment.
// 1. cont, if leader fails -> then backup forward to candidate no matter what.
// 1. cont, if leader, simply forward to kubernetes endpoints.
// 2. Get service ports and start to listen on those ports. (for incoming requests to k8 cluster.).
// 3. If leader, get the different node ips that are active on the kbuernetes cluster and proxy to one of them.
// Basically this proxy will act as the entrypoint / increased security & service discovery etc.

type Proxy struct{
	addr string
	leaderChan <-chan string 
	leaderAddr string
	registerPorts <-chan []int // Register new port from kubernetes cluster. Same channels used for deleting channels.
	registerNodes <-chan []string // Register kubernetes nodes.
	activePorts map[int]*net.TCPListener
	portsMut sync.Mutex
	activeNodes []string
}

func InitProxy(leaderCh <-chan string, portChan <- chan []int, addr string) *Proxy{
	return &Proxy{leaderChan: leaderCh, 
		leaderAddr: "", 
		registerPorts: portChan, 
		activePorts: make(map[int](*net.TCPListener)),
		addr: addr,
		activeNodes: []string{},
		portsMut: sync.Mutex{},
	}
}

func (p *Proxy) Start(){
	log.Println("Starting proxy for: ", p.addr)
	for{
		select{
		case prt := <-p.registerPorts:
		p.checkNewPorts(prt)
		case address := <-p.leaderChan:
		p.leaderAddr = address
		case clusterNodes := <-p.registerNodes:
		p.activeNodes = clusterNodes
		}
	}
}

func (p *Proxy) checkNewPorts(prts []int){
	// If exists in prts and activePorts - do nothing.
	// If exists in prts and not in activePorts - call registerNewPort(prt)
	for _, port := range prts{
		p.portsMut.Lock()
		_, exist := p.activePorts[port]
		p.portsMut.Unlock()
		if exist{
			log.Println(p.addr, "PORT ALREADY EXIST AND RUNNING:", port)
		}else{
			go p.registerNewPort(port)
		}
	}
	// If exist in activePorts and not in ports - call to terminate port. 
	p.portsMut.Lock()
	defer p.portsMut.Unlock() 
	for ip, ss := range p.activePorts{
		if (!exist(ip, prts)){
			log.Println(p.addr,": REMOVING SOCKET:", ip)
			ss.Close()
			delete(p.activePorts, ip)
		}
	}
}

func exist(a int, b []int) bool{
	for _, c := range b{
		if a == c {
			return true
		}
	}
	return false
}


// NOTE: Starting to listen but not clossing. 
func (p *Proxy) registerNewPort(port int){
	p.portsMut.Lock()
	ss, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP(p.addr), Port:port})
	// Add server socket to object dict, if close simply close.
	if err != nil{
		log.Println("FAILED TO LISTEN ON PORT",port)
	        p.portsMut.Unlock()
		return
	}
	p.activePorts[port] = ss
	p.portsMut.Unlock()
	log.Println("PROXY: ",p.addr," - REGISTERED NEW PORT FOR PROXY:", port)
	for {
		s, err := ss.Accept()
		if err != nil{
			// NOTE: Need to only return on closed socket error, other missreads/timeouts etc should not stop listening port.
			log.Printf("FAILED TO READ SOCKET %s", err)
			return
		}
		go p.forwardToLeader(s, port)
	}
}

func (p *Proxy) forwardToLeader(inc net.Conn, port int){
	defer inc.Close()
	if p.leaderAddr == p.addr {
		p.logProxy(inc.LocalAddr().String(), "KUB CLUSTER")
		buffer := make([]byte, 2048)
		n, err := inc.Read(buffer)
		if err != nil{
			log.Println("Failed to read from INC TO CLUSTER")
			return
		}
		err, resp := p.forwardToCluster(buffer[:n], port)
		if err != nil{
			log.Println("FAILED TO GET RESPONSE FROM CLUSTER. ",err)
			return
		}
		p.logProxy(p.addr, inc.LocalAddr().String())
		_, err = inc.Write(resp)
		if err != nil{
			log.Println("FAILED TO WRITE TO CONNECTION, REASON:", err)
		}
		return
	}else{
		leaderConn, err := net.DialTCP("tcp", nil, &net.TCPAddr{IP:net.ParseIP(p.leaderAddr),Port: port})
		if err != nil{
			// TODO Insert retries. 
			p.logProxy(p.addr, inc.LocalAddr().String())
			inc.Write([]byte("ERROR WRITING TO LEADER."))
			return
		}
		// TODO: Insert some error handling. 
		buffer := make([]byte, 2048)
		n, err := inc.Read(buffer)
		if err != nil{
			log.Println("FAILED TO READ FROM CONN", err)
		} 
		p.logProxy(inc.LocalAddr().String(), p.addr)
		// TODO: Make some L7 filtering. 
		p.logProxy(p.addr, p.leaderAddr+":"+strconv.Itoa(port))
		_, err = leaderConn.Write(buffer[:n])
		if err != nil{
			log.Println("FAILED TO  WRITE TO LEADER", err)
		} 
		// Await response.
		n, err = leaderConn.Read(buffer)
		if err != nil{
			log.Println("FAILED TO READ FROM LEADER", err)
		} 
		p.logProxy(p.leaderAddr+":"+strconv.Itoa(port), p.addr)
		// Return response to caller.
		p.logProxy(p.addr, inc.LocalAddr().String())
		_, err = inc.Write(buffer[:n])
		if err != nil{
			log.Println("FAILED TO WRITE FROM PROXY CALLER", err)
		} 
	}
}


// Will pick one of the kubeNodes and randomly forward to that node.
func (p *Proxy) forwardToCluster(packet []byte, kubePort int) (error, []byte){ 
	p.logProxy(p.addr+":"+strconv.Itoa(kubePort), "TODO_SELECT_KUBE_NODE"+":"+strconv.Itoa(kubePort))
	nodes := p.activeNodes

	// TODO: Decide on loadbalancing strategy /retries etc.
	// For now just random load balancing. 
	for n := range nodes{
		i := rand.Intn(n+1)
		nodes[n],nodes[i] = nodes[i], nodes[n] 
	}
	for _, node := range nodes{
		log.Println("Calling node: ",node)
		kubResp := make(chan []byte) 
	        go func(respCh chan []byte){
			kubeCon, err := net.DialTCP("tcp", nil, &net.TCPAddr{IP:net.ParseIP(node),Port: kubePort})
			if err != nil{
				log.Println("Failed to call node: ", node)
				return
			}
			_, err = kubeCon.Write(packet)
			if err != nil{
				log.Println("FAILED TO WRITE TO ESTABLISHED KUBE SOCKET")
				return
			}
			respBuffer := make([]byte, 2048)
			n, err := kubeCon.Read(respBuffer)
			if err != nil{
				log.Println("FAILED TO READ FROM ESTABLISHED KUBE SOCKET")
				return
			}
			kubResp<-respBuffer[:n]
		}(kubResp)
		select{
		case <-time.After(time.Second*2):
		log.Println("NODE FAILURE: "+node)
		case resp := <- kubResp:
		log.Println("GOT RESPONSE FROM NODE:", string(resp))
		return nil, resp
		}
	}
	return fmt.Errorf("FAILED TO REACH ANY OF THE KUB NODES."),[]byte{}
}

func (p *Proxy) logProxy(src string, dst string){
	if src == p.addr{
		src = "SELF"
	}else if dst == p.addr{
		dst = "SELF"
	}
	log.Println(fmt.Sprintf("PROXY: %s -> %s", src, dst))
}


