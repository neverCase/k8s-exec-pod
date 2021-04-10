package main

import (
	"context"
	"flag"
	"github.com/Shanghai-Lunara/pkg/zaplogger"
	"github.com/nevercase/k8s-controller-custom-resource/pkg/signals"
	exec "github.com/nevercase/k8s-exec-pod"
)

func main() {
	var kubeconfig = flag.String("kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	var masterUrl = flag.String("master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	var proxyservice = flag.String("proxyservice", "0.0.0.0:9090", "The address of the http server.")
	flag.Parse()
	defer zaplogger.Sync()
	stopCh := signals.SetupSignalHandler()
	_ = exec.InitServer(context.Background(), *proxyservice, *kubeconfig, *masterUrl)
	<-stopCh

	<-stopCh
}
