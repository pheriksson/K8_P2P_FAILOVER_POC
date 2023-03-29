package kube; 

// kube control interface - any k8 related stuff.
// Need config object - (basically location of kubectl auth token, or dont really need kubectl token as we can talk to control plane via simple json objects.)
// But we need location of cp node (and alternativly other nodes if cp we're to be down. or simply asume if other nodes are down that cp will handle it).

type KubeClient struct{
	clusterInput chan string
	pocCommands chan string
}

func InitKubeClient() *KubeClient{
	return &KubeClient{make(chan string), make(chan string)};
}


// Heartbeat on kube control plane - getting metrics.


// Start cluster from etcd instance.


// Shutdown control plane as other request taking over.




