package k8s_exec_pod

import (
	"context"
	"github.com/Shanghai-Lunara/pkg/zaplogger"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
)

func openStream(k8sClient kubernetes.Interface, option *ExecOptions) (io.ReadCloser, error) {
	rc, err := k8sClient.CoreV1().RESTClient().Get().
		Resource("pods").
		Namespace(option.Namespace).
		Name(option.PodName).
		SubResource("log").
		VersionedParams(&corev1.PodLogOptions{
			Container:    option.ContainerName,
			Follow:       option.Follow,
			Previous:     option.UsePreviousLogs,
			Timestamps:   false,
			SinceSeconds: option.SinceSeconds,
			//SinceTime:    option.SinceTime,
		}, scheme.ParameterCodec).Stream(context.Background())
	return rc, err
}

func LogTransmit(k8sClient kubernetes.Interface, session Session) error {
	readCloser, err := openStream(k8sClient, session.Option())
	if err != nil {
		zaplogger.Sugar().Error(err)
		session.Close(err.Error())
		return err
	}
	session.ReadCloser(readCloser)
	defer func() {
		zaplogger.Sugar().Info("LogTransmit readCloser close")
		if err := readCloser.Close(); err != nil {
			zaplogger.Sugar().Error(err)
		}
	}()
	zaplogger.Sugar().Info("LogTransmit io.Copy start session:", session.Id())
	if _, err = io.Copy(session, readCloser); err != nil {
		session.Close(err.Error())
		return err
	}
	zaplogger.Sugar().Infof("LogTransmit trigger session.Close session:%s reason:%s", session.Id(), ReasonStreamStopped)
	session.Close(ReasonStreamStopped)
	return nil
}

func LogDownload(k8sClient kubernetes.Interface, option *ExecOptions) (io.ReadCloser, error) {
	return openStream(k8sClient, option)
}
