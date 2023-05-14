package proxy

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

const(
// Const port number used to test cluster on local environment as nodeports
// are accessible through all nodes in cluster. meaning solution will simply be
// to shit the proxy port for X amount. meaning a req to nodeport X will be reachable
// from proxy port X+LOCAL_DEV_PORT_PROXY_SHIFT, 
	LOCAL_DEV_PORT_PROXY_SHIFT=100
	TIME_SEC_AWAIT_KUBE_CLUSTER_TIMEOUT=2
)

type Proxy struct{
	addr string
	port int
	peerNodes []string 
	clusterPorts <-chan []int 
	clusterNodes <-chan []string 
	activeClusterPorts map[int]*net.TCPListener
	activeClusterNodes []string
	portsMut sync.Mutex
	nodesMut sync.Mutex
}



func InitProxy(ip string, port int, clusterNodes <-chan []string, clusterPorts <-chan []int) *Proxy{
	return &Proxy{
		addr: ip,
		port: port,
		peerNodes: []string{},
		clusterNodes: clusterNodes,
		clusterPorts: clusterPorts, 
		activeClusterPorts: make(map[int](*net.TCPListener)),
		activeClusterNodes: []string{},
		portsMut: sync.Mutex{},
	}
}


func (p *Proxy) Start(peers []string){
	for _, peer := range peers{
		p.peerNodes = append(p.peerNodes, peer)
	}
	p.logProxy(fmt.Sprintf("STARTING ON [%s:%d]", p.addr, p.port))
	go p.listenProxies()
	for{
		select{
		case prts := <-p.clusterPorts:
			//p.logProxy("RECIEVED CLUSTER PORTS", fmt.Sprint(prts))
			p.checkNewPorts(prts)
		case nodes := <-p.clusterNodes:
			//p.logProxy("RECIEVED CLUSTER NODES", fmt.Sprint(nodes))
			p.nodesMut.Lock()
			p.activeClusterNodes = nodes
			p.nodesMut.Unlock()
		}
	}
}


func (p *Proxy) checkNewPorts(prts []int){
	// NOTE THE USE OF PORT SKEW TO IMITATE PROXY REQUESTS AND PROXY FORWARDING.
	// SAVING PORTS AT SKEW, AND FORWARDING TO CLUSTER AT PORT-LOCAL_DEV_PORT_PROXY_SHIFT 
	for _, port := range prts{
		if port == 0 {continue}
		port = port+LOCAL_DEV_PORT_PROXY_SHIFT
		p.portsMut.Lock()
		_, exist := p.activeClusterPorts[port]
		p.portsMut.Unlock()
		if exist{
			//p.logProxy("PORT ALREADY EXIST AND RUNNING", fmt.Sprint(port))
		}else{
			go p.listenPort(port)
		}
	}

	p.portsMut.Lock()
	defer p.portsMut.Unlock() 
	for port, ss := range p.activeClusterPorts{
		if (!exist(port-LOCAL_DEV_PORT_PROXY_SHIFT, prts)){
			p.logProxy(fmt.Sprintf("%s REMOVING SOCKET [%d]", p.addr, port+LOCAL_DEV_PORT_PROXY_SHIFT))
			ss.Close()
			delete(p.activeClusterPorts, port+LOCAL_DEV_PORT_PROXY_SHIFT)
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

func (p *Proxy) listenProxies(){
	ss, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP(p.addr), Port:p.port})	
	if err != nil{
		log.Panic("FAILED TO LISTEN ON PORT", p.port)
	}
	for {
		s, err := ss.Accept()
		if err != nil{
			log.Println("Failed to read inc request:", err)
			return
		}
		go func(s net.Conn){
			pp, err := ReadProxyPacket(s)
			if err != nil{
				log.Println("FAILED TO READ PROXY PACKET FROM CALLER", s.LocalAddr())
				return
			}
			err, resp := p.forwardToCluster(pp.Data, pp.Port)
			if err != nil{
				p.logProxy("INC PEER PROXY REQUEST FAILED - RETURNING FAILURE TO PEER PROXY")
				err = WriteProxyPacket(s, ProxyPacket{Failed: true, Port: pp.Port})
			}else{
				//p.logProxy("INC PEER PROXY REQUEST SUCCESS - RETURNING DATA TO PEER PROXY")
				err = WriteProxyPacket(s, ProxyPacket{Failed: false, Data: resp, Port: pp.Port})

			}
		}(s)
	}
}

func (p *Proxy) listenPort(port int){
	p.portsMut.Lock()
	ss, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP(p.addr), Port:int(port)})
	// Add server socket to object dict, if close simply close.
	if err != nil{
		p.logProxy(fmt.Sprintf("FAILED TO LISTEN AT PORT [%d]", port))
	        p.portsMut.Unlock()
		return
	}
	p.activeClusterPorts[port] = ss
	p.portsMut.Unlock()
	p.logProxy(fmt.Sprintf("REGISTERED NEW PORT [%s:%d]",p.addr, port))
	for {
		s, err := ss.Accept()
		if err != nil{
			closedConn, ok := err.(*net.OpError)
			if ok && closedConn.Err.Error() == "use of closed network connection"{
				//p.logProxy(fmt.Sprintf("SUCCESFULLY CLOSED PORT: [%d]", port))
			}else{
				p.logProxy(fmt.Sprintf("FAILED TO READ SOCKET FROM [%s]", s.LocalAddr().String()))
			}
			return
		}
		go func(clusterPort int){
			defer s.Close()
			buffer := make([]byte, 2048)
			n, err := s.Read(buffer)
			if err != nil{
				p.logProxy(fmt.Sprintf("FAILED TO READ INC REQUEST ON PORT [%d]", clusterPort))
				return 
			}
			payload := buffer[:n]
			err, resp := p.forwardToCluster(payload, clusterPort)
			if err == nil{
				// Local cluster request success.
				_, err = s.Write(resp)
				if err != nil{
					p.logProxy(fmt.Sprintf("FAILED TO RESPOND TO SUCCESFULL KUB CLUSTER REQUEST ON PORT [%d] - MSG:\n[%s]", clusterPort, payload))
				}else{
				        //p.logProxy(fmt.Sprintf("SUCCESFULLY COMPLETED PROXY REQUEST ON PORT [%d]", clusterPort))
				}
				return 
			}
			// Err from local cluster, proxy 
			err, resp = p.proxyRequest(payload, clusterPort)
			if err != nil{
				p.logProxy(fmt.Sprintf("FAILED TO PROXY REQUEST TO LOCAL CLUSTER [%s]", err))
				s.Write([]byte("FAILED PROXY REQUEST"))
				return 
			}
			//p.logProxy(fmt.Sprintf("PROXY SUCCESS ON [%d]", clusterPort))
			_, err = s.Write(resp)
			if err != nil{
				p.logProxy("FAILED TO RESPOND TO SUCCESSFULL PROXY REQUEST - REASON:", err.Error())
			}
		}(int(port))
	}
}

func (p *Proxy) getRandomPeerAddr() string{
	for n := range p.peerNodes{
		i := rand.Intn(n+1)
		p.peerNodes[n], p.peerNodes[i] = p.peerNodes[i], p.peerNodes[n]
	}
	for _, peer := range p.peerNodes{
		if peer != "" {
			return peer
		}
	}
	return ""
}

func (p *Proxy) proxyRequest(data []byte, clusterPort int) (error, []byte){
	peer := p.getRandomPeerAddr()
	//p.logProxy(fmt.Sprintf("PROXY REQUEST TO PEER [%s]", peer))
	s, err := net.DialTCP("tcp",nil,&net.TCPAddr{IP:net.ParseIP(peer), Port: p.port})
	if err != nil{
		return err, data
	}
	err = WriteProxyPacket(s, ProxyPacket{Data:data, Port: clusterPort})
	if err != nil{
		return fmt.Errorf("Failed to write packet to proxy"+err.Error()), []byte{}
	}
	pp, err := ReadProxyPacket(s)
	if pp.Failed || err != nil {
		return fmt.Errorf("Failed to proxy packet"), []byte{}
	}
	return nil, pp.Data
}

// loadbalance nodeport service on local cluster. 
func (p *Proxy) forwardToCluster(packet []byte, kubePort int) (error, []byte){ 
	kubePort = kubePort-LOCAL_DEV_PORT_PROXY_SHIFT
	p.nodesMut.Lock()
	nodes := make([]string, len(p.activeClusterNodes))
	copy(nodes, p.activeClusterNodes)
	p.nodesMut.Unlock()
	for n := range nodes{
		i := rand.Intn(n+1)
		nodes[n],nodes[i] = nodes[i], nodes[n] 
	}
	for _, node := range nodes{
		if node == "" {continue}
		//p.logProxy(fmt.Sprintf("CLUSTER NODE [%s:%d]", node, kubePort))
		kubResp := make(chan []byte)
		failure := make(chan bool)
	        go func(respCh chan []byte, done chan bool){
			kubeCon, err := net.DialTCP("tcp", nil, &net.TCPAddr{IP:net.ParseIP(node),Port: kubePort})
			if err != nil{
				done<-true
				return
			}
			_, err = kubeCon.Write(packet)
			if err != nil{
				done<-true
				return
			}
			respBuffer := make([]byte, 2048)
			n, err := kubeCon.Read(respBuffer)
			if err != nil{
				done<-true
				return
			}
			kubResp<-respBuffer[:n]
		}(kubResp, failure)
		select{
		case <-failure:
		        //p.logProxy(fmt.Sprintf("LOCATION CLUSTER NODE FAILURE"))
			close(kubResp)
			return fmt.Errorf("FAILED TO REACH ANY OF THE KUB NODES."),[]byte{}
		case <-time.After(time.Second*TIME_SEC_AWAIT_KUBE_CLUSTER_TIMEOUT):
		        //p.logProxy(fmt.Sprintf("LOCATION CLUSTER NODE FAILURE - TIMEOUT"))
			close(kubResp)
			return fmt.Errorf("FAILED TO REACH ANY OF THE KUB NODES."),[]byte{}
		case resp := <- kubResp:
		        //p.logProxy(fmt.Sprintf("RECIEVED LOCATION CLUSTER NODE RESPONSE"))
			return nil, resp
		}
	}
	return fmt.Errorf("FAILED TO REACH ANY OF THE KUB NODES."),[]byte{}
}

func (p *Proxy) logProxy(msgs ...string){
	conc := ""
	for _, msg := range msgs {
		conc+=" "+msg
	} 
	log.Println(fmt.Sprintf("[PROXY]: %s", conc))
}


