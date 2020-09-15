package k8s_exec_pod

import (
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
)

func openStream(k8sClient kubernetes.Interface, option ExecOptions) (io.ReadCloser, error) {
	return k8sClient.CoreV1().RESTClient().Get().
		Resource("pods").
		Namespace(option.Namespace).
		Name(option.PodName).
		SubResource("log").
		VersionedParams(&corev1.PodLogOptions{
			Container:  option.ContainerName,
			Follow:     true,
			Previous:   option.UsePreviousLogs,
			Timestamps: true,
		}, scheme.ParameterCodec).Stream()
}
