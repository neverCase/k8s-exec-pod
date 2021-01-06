package k8s_exec_pod

import (
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
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
			Timestamps: false,
		}, scheme.ParameterCodec).Stream()
}

func LogTransmit(k8sClient kubernetes.Interface, session Session) error {
	readCloser, err := openStream(k8sClient, session.Option())
	if err != nil {
		klog.V(2).Info(err)
		session.Close(err.Error())
		return err
	}
	session.ReadCloser(readCloser)
	defer func() {
		klog.Info("LogTransmit readCloser close")
		if err := readCloser.Close(); err != nil {
			klog.V(2).Info(err)
		}
	}()
	klog.Info("LogTransmit io.Copy start session:", session.Id())
	if _, err = io.Copy(session, readCloser); err != nil {
		session.Close(err.Error())
		return err
	}
	klog.Infof("LogTransmit trigger session.Close session:%s reason:%s", session.Id(), ReasonStreamStopped)
	session.Close(ReasonStreamStopped)
	return nil
}
