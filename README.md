# K8s geographical failover POC 

POC that deals with kubernetes geographical failover. The POC handles
failovers externally (outside the cluster) by maintaining fully replicated cluster object logs, meaning
no external configuration is required for failovers dependent on service locality.


# Prerequisites:
 - At least two (fresh) clusters with network connectivity.
 - go1.19.1

# Build
:warning: if running POC on a cluster node, specify port shift in file **proxy/proxy.go** (const: 'LOCAL_DEV_PORT_PROXY_SHIFT') to be able to access proxy for failover capabilities.

Build from src:
```
go build .
```
Run for each cluster:
```
./K8_P2P_FAOLOVER_POC -ip=x.x.x.x -peerips=y.y.y.y,z.z.z.z
```

# Usage
In either TUI, use the following commands:
 - 1: Create deployment object of POC [image](https://hub.docker.com/repository/docker/seaweed39kelp/poctesting/general)
 - 2: Create nodeport resrouce for deployment in **1**


