package main

// This is NOT a tut0riaI how to write connection pool or use k8s API
// This is a program to trigger a memory leak in kubernetes/client-go package
// It is intentionally broken
// (and because I wrote this in a hurry, it might be also unintentionally broken)

import (
	"context"
	"flag"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const kubeConfigEnvName = "KUBECONFIG"

var defaultDialContextFunc = (&net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: -30 * time.Second,
}).DialContext

type MyK8sApiWrapper struct {
	kubeConfig string
	restConfig *rest.Config
	transport  *http.Transport
	clientSet  kubernetes.Interface

	lock   sync.RWMutex
	active bool
}

func NewK8sApiWrapper(kubeConfig string) (*MyK8sApiWrapper, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}

	w := &MyK8sApiWrapper{
		kubeConfig: kubeConfig,
		restConfig: config,
		active:     false,
	}
	// This breaks vendor/k8s.io/client-go/transport/cache.go
	// Actual problem is triggered in GetClientSet function below
	w.restConfig.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		con, err := defaultDialContextFunc(ctx, network, address)
		// at this level we can only deactivate the client set if connection failed
		if err != nil {
			w.lock.Lock()
			w.active = false
			w.lock.Unlock()
		}
		return con, err
	}
	w.lock.Lock()
	defer w.lock.Unlock()
	w.clientSet, err = kubernetes.NewForConfig(w.restConfig)
	if err != nil {
		return nil, err
	}
	w.active = true
	return w, nil
}

func (w *MyK8sApiWrapper) GetClientSet() (kubernetes.Interface, error) {
	// Assume our wrapper is broken for some reason
	//	if w.active {
	//	return w.clientSet, nil
	// }

	// Try to reconnect
	var err error
	w.lock.Lock()
	defer w.lock.Unlock()
	// Every time this is called, a new entry in k8s.io/client-go/transport.tlsTransport.transports map is created
	// As of k8s.io/client-go v0.27.3, this map is never pruned, producing [potentially] infinite memory leak
	w.clientSet, err = kubernetes.NewForConfig(w.restConfig)
	if err != nil {
		return nil, err
	}
	w.active = true
	return w.clientSet, nil
}

func main() {
	var pprofAddress *string
	pprofAddress = flag.String("pprof", "", "IP:port for pprof endpoint, e.q. 127.0.0.1:21285")
	var kubeconfig *string
	var sleepSeconds *int
	sleepSeconds = flag.Int("sleep", 40, "Seconds to sleep between queries")

	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	flag.Parse()
	if *pprofAddress != "" {
		go func() {
			fmt.Fprintf(os.Stderr, "start listening for profiling/debug requests on %v", *pprofAddress)
			profilingServer := &http.Server{Addr: *pprofAddress}
			err := profilingServer.ListenAndServe()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error listening profiling API: %v", err)
			}
		}()
	}
	kubeconfigEnv := os.Getenv(kubeConfigEnvName)
	if kubeconfigEnv != "" {
		kubeconfig = &kubeconfigEnv
	}
	myWrapper, err := NewK8sApiWrapper(*kubeconfig)
	if err != nil {
		panic(err)
	}

	// main loop
	for {
		client, err := myWrapper.GetClientSet()
		if err != nil {
			panic(err)
		}
		nodes, err := client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			panic(err)
		}

		for _, node := range nodes.Items {
			fmt.Printf("%s\n", node.Name)
			for _, condition := range node.Status.Conditions {
				fmt.Printf("\t%s: %s\n", condition.Type, condition.Status)
			}
		}

		time.Sleep(time.Duration(*sleepSeconds) * time.Second)
	}
}
