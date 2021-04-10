package k8s_exec_pod

import (
	"github.com/Shanghai-Lunara/pkg/zaplogger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func NewResource(masterUrl, kubeconfigPath string) (*rest.Config, kubernetes.Interface) {
	cfg, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
	if err != nil {
		zaplogger.Sugar().Fatal("Error building kubeconfig: %s", err.Error())
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		zaplogger.Sugar().Fatal("Error building kubernetes clientset: %s", err.Error())
	}
	return cfg, kubeClient
}
