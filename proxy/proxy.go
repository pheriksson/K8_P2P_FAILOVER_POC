package proxy

import (
	"fmt"
	"log"
	"bytes"
	"encoding/gob"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"
)

// TODO: Rename to ip or address accross all files.
// TODO: Refactor variable race conditions. Separate to leader mut, and then have interface shared resource which includes, equals, write, read. And do that for each single shared variable.
// GetActiveNodes from leader - for loadbalancing.
// GetActiveLeader - If self -> simply process request to nodes // else forward to leader proxy.
// GetActivePorts - basically used to get the accessible ports, start to listen on each of the accessible ports.

// 1. Forward request to leader, if in failover mode, still forward to leader as candidate will assume leader upon active k8 deployment.
// 1. cont, if leader fails -> then backup forward to candidate no matter what.
// 1. cont, if leader, simply forward to kubernetes endpoints.
// 2. Get service ports and start to listen on those ports. (for incoming requests to k8 cluster.).
// 3. If leader, get the different node ips that are active on the kbuernetes cluster and proxy to one of them.
// Basically this proxy will act as the entrypoint / increased security & service discovery etc.
const(
	// TODO, change port placement of all different services to some local config file.
	PROXY_PORT=9996
	TIME_SEC_QUERY_SVC_PORTS=10
)


// External ingress for cluster.
type Proxy struct{
	addr string
	leaderAddr string
	leaderChan <-chan string 
	leaderMut sync.Mutex
	registerPorts <-chan []int // Register new port from kubernetes cluster. Same channels used for deleting channels.
	registerNodes <-chan []string // Register kubernetes nodes.
	activePorts map[ProxyPort]*net.TCPListener
	portsMut sync.Mutex
	activeNodes []string
}



func InitProxy(activeLeader <-chan string, fromKubeIP <-chan []string, fromKubePort <-chan []int, ip string) *Proxy{
	return &Proxy{leaderChan: activeLeader, 
		leaderAddr: "", 
		registerPorts: fromKubePort, 
		activePorts: make(map[ProxyPort](*net.TCPListener)),
		addr: ip,
		registerNodes: fromKubeIP,
		activeNodes: []string{},
		portsMut: sync.Mutex{},
		leaderMut : sync.Mutex{},
	}
}



func (p *Proxy) isLeader() bool{
	p.leaderMut.Lock()
	defer p.leaderMut.Unlock()
	return p.addr == p.leaderAddr
}

func (p *Proxy) getLeader() string{
	p.leaderMut.Lock()
	defer p.leaderMut.Unlock()
	return p.leaderAddr 
}

func (p *Proxy) Start(){
	log.Println("Starting proxy for: ", p.addr)
	fromProxyLeaderPorts := p.requestLeaderProxyPorts()
	for{
		select{
		case prts := <-fromProxyLeaderPorts:
			// If not leader, meaning we got valid service port update.
			log.Println("RECIEVED PORTS FROM LEADER PROXY")
			if !p.isLeader() {p.checkNewPorts(prts)}
		case prts := <-p.registerPorts:
			log.Println("RECIEVED PORTS FROM LOCAL KUBE CLUSTER", prts)
			if p.isLeader() {
				refactor := make([]ProxyPort, len(prts))
				for _, prt  := range prts{
					if prt != 0{
						refactor = append(refactor, ProxyPort(prt))
					}
				}
				p.checkNewPorts(refactor)
			}
		case newLeader := <-p.leaderChan:
			p.leaderMut.Lock()
			p.leaderAddr = newLeader
			p.leaderMut.Unlock()
		case p.activeNodes = <-p.registerNodes:
			log.Println("RECIEVED NEW NODE ENDPOINTS FROM LOCAL CLUSTER:", p.activeNodes)
		}
	}
}

// Handles service discovery for proxying peers requesting current leader (detemrined by Modified RAFT in PoC)
func (p *Proxy) requestLeaderProxyPorts() chan ProxyPorts{ 
	proxyLeaderPorts := make(chan ProxyPorts) 
	go func(){
		for{
		        time.Sleep(time.Second*TIME_SEC_QUERY_SVC_PORTS)
			if ldr:= p.getLeader(); !p.isLeader(){
				log.Println("REQUESTING SERVICES FROM LEADER")
				conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{IP:net.ParseIP(ldr), Port: PROXY_PORT})
				buffer := make([]byte, 2048)
				n, err := conn.Read(buffer)
				if err != nil{
					log.Println("FAILED TO QUERY LEADER FOR SVC PORTS.")
				}
				buffer = buffer[:n]
				log.Println("TRYING TO DECODE BUFFER:")
				pp, err := decodeProxyPorts(buffer)
				if err != nil{
					log.Println("FAILED TO DECODE THE PROXY PORTS")
				}
				log.Println("RECIEVED THE FOLLOWING PORTS FROM LEADER:", pp)
				proxyLeaderPorts<-pp
			}
		}
	}()
	go func(){
		proxySocket, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP(p.addr),Port: PROXY_PORT})
		defer proxySocket.Close()
		if err != nil {
			log.Panic("FAILED TO INITIALIZE PROXY ON PORT:", PROXY_PORT, err.Error())
		}
		for {
			conn, err := proxySocket.Accept()
			if err != nil {
				log.Println("FAILED TO READ ON PROXY_SOCKET:", err)
			}
			svcPorts := p.getServicePorts()
			go func(){
				// Make sure connection closes. 
				defer conn.Close()
			        data, err := svcPorts.encode()
				if err != nil{
					log.Println("FAILED TO ENCODE:", svcPorts)
				}
			        _, err = conn.Write(data)
			        if err != nil {
				    log.Println("FAILED TO RETURN SVC PORTS")
			        }
			}()
		}
	}()
	return proxyLeaderPorts
}

func (p *Proxy) getServicePorts() ProxyPorts{
	ports := make(ProxyPorts, len(p.activePorts))
	for port, _ := range p.activePorts{
		ports = append(ports, ProxyPort(port))
	}
	return ports
}

func (p *Proxy) checkNewPorts(prts ProxyPorts){
	for _, port := range prts{
		if port == 0 {continue}
		p.portsMut.Lock()
		_, exist := p.activePorts[port]
		p.portsMut.Unlock()
		if exist{
			log.Println(p.addr, "PORT ALREADY EXIST AND RUNNING:", port)
		}else{
			go p.listenPort(port)
		}
	}

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

func exist(a ProxyPort, b ProxyPorts) bool{
	for _, c := range b{
		if a == c {
			return true
		}
	}
	return false
}



func (p *Proxy) listenPort(port ProxyPort){
	p.portsMut.Lock()
	ss, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP(p.addr), Port:int(port)})
	// Add server socket to object dict, if close simply close.
	if err != nil{
		log.Println("FAILED TO LISTEN ON PORT",port)
	        p.portsMut.Unlock()
		return
	}
	p.activePorts[port] = ss
	p.portsMut.Unlock()
	log.Println(fmt.Sprintf("REGISTERED NEW PORT [%d]",port))
	for {
		s, err := ss.Accept()
		if err != nil{
			closedConn, ok := err.(*net.OpError)
			if ok && closedConn.Err.Error() == "use of closed network connection"{
				log.Println("SUCCESFULLY CLOSED PORT:", port)
			}else{
				log.Printf("FAILED TO READ SOCKET %s", err)
			}
			return
		}
		go p.forwardToLeader(s, port)
	}
}

// ProxyRequest {From: net.Conn, targetPort: ProxyPort}
func (p *Proxy) forwardToLeader(inc net.Conn, port ProxyPort){
	defer inc.Close()
	ldr := p.getLeader()
	from := inc.LocalAddr().String()
	if p.isLeader(){
		self := ldr
		p.logProxy(from, self)
		buffer := make([]byte, 2048)
		n, err := inc.Read(buffer)
		if err != nil{
			log.Println("Failed to read from INC TO CLUSTER")
			return
		}
		p.logProxy(self,  "CLUSTER")
		err, resp := p.forwardToCluster(buffer[:n], int(port))
		if err != nil{
			log.Println("FAILED TO GET RESPONSE FROM CLUSTER. ",err)
			return
		}
		p.logProxy(self, from)
		_, err = inc.Write(resp)
		if err != nil{
			log.Println("FAILED TO WRITE TO CONNECTION, REASON:", err)
		}
		return
	}else{
		// Rename to ldr, self, from.
		self := p.addr; 
		leaderConn, err := net.DialTCP("tcp", nil, &net.TCPAddr{IP:net.ParseIP(ldr),Port: int(port)})
		
		if err != nil{
			// TODO Insert retries. 
			p.logProxy(self,  from)
			inc.Write([]byte("ERROR WRITING TO LEADER."))
			return
		}
		// TODO: Insert some error handling. 
		buffer := make([]byte, 2048)
		p.logProxy(from, self)
		n, err := inc.Read(buffer)
		if err != nil{
			log.Println("FAILED TO READ FROM CONN", err)
		} 
		// TODO: Make some L7 filtering. 
		p.logProxy(self,ldr+":"+strconv.Itoa(int(port)))
		_, err = leaderConn.Write(buffer[:n])
		if err != nil{
			log.Println("FAILED TO  WRITE TO LEADER", err)
		} 
		// Await response.
		p.logProxy(p.leaderAddr+":"+strconv.Itoa(int(port)), self)
		n, err = leaderConn.Read(buffer)
		if err != nil{
			log.Println("FAILED TO READ FROM LEADER", err)
		} 
		// Return response to caller.
		p.logProxy(self, from)
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
		if node == "" {continue}
		log.Println("PROXY: Calling node: ",node, "at port: ", kubePort)
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

type ProxyPorts []ProxyPort
type ProxyPort int

func (p *ProxyPorts) encode() ([]byte, error){
	var buffer bytes.Buffer
	enc := gob.NewEncoder(&buffer);
	err := enc.Encode(p)
	if err != nil{
		log.Printf("ENCODE ERROR: %s",err)
		return nil, nil
	}
	byteBuff := make([]byte, 2048)
	n, _ := buffer.Read(byteBuff);
	return byteBuff[:n], nil
}

func decodeProxyPorts(proxyPorts []byte) (ProxyPorts, error){
	buffer := bytes.NewBuffer(proxyPorts)
	dec := gob.NewDecoder(buffer)
	var packet ProxyPorts
	err := dec.Decode(&packet);
	if err != nil{
		log.Printf("DECODE ERROR: %s", err)
		return packet, err
	}
	return packet, nil;

}

func (p *Proxy) logProxy(src string, dst string){
	if src == p.addr{
		src = "SELF"
	}else if dst == p.addr{
		dst = "SELF"
	}
	log.Println(fmt.Sprintf("PROXY: %s -> %s", src, dst))
}


