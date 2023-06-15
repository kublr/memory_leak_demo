# memory_leak_demo
A demo program to trigger a memory leak in k8s.io/client-go v0.27.x and (somewhat) older.

**CAUTION:** this is not a tut0riaI on making a connection pool or using k8s API. 
This program is intentionally broken (and because I wrote it in a hurry, it might be also unintentionally broken). 

If you create a rest.Config with custom Dial function, it breaks the cache in k8s.io/client-go/transport/cache.go.
The cache creates a new entry every time you request a new ClientSet with this config, 
resulting in potentially infinite memory leak.

build with `go build memory_leak_demo`.

Options:
-kubeconfig filename: A path to valid kubeconfig file for a running Kubernetes cluster.  
Kubeconfig needs the permission to list cluster nodes.  Default ~/.kube/config.
Also can be set via KUBECONFIG env variable.

-pprof ip:port: A TCP port to listen for pprof HTTP requests, e.q. 127.0.0.1:21285.  
See https://pkg.go.dev/net/http/pprof how to use this endpoint.  This tool helps to verify the leak.  
Default: HTTP pprof disabled.

-sleep seconds: Interval between k8s API queries. Smaller interval means faster leak.
