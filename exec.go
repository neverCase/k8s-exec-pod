package k8s_exec_pod

import (
	"github.com/Shanghai-Lunara/pkg/zaplogger"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// isValidShell checks if the shell is an allowed one
func isValidShell(validShells []string, shell string) bool {
	zaplogger.Sugar().Infow("isValidShell", "shell", shell)
	for _, validShell := range validShells {
		if validShell == shell {
			return true
		}
	}
	return false
}

// Terminal is called from Session as a goroutine
// Waits for the websocket connection to be opened by the client the session to be bound in Session.HandleProxy
func Terminal(k8sClient kubernetes.Interface, cfg *rest.Config, session Session) {
	var err error
	validShells := []string{"bash", "sh", "powershell", "cmd"}

	if isValidShell(validShells, session.Option().Command[0]) {
		err = Exec(k8sClient, cfg, session)
	} else {
		// No shell given or it was not valid: try some shells until one succeeds or all fail
		// FIXME: if the first shell fails then the first keyboard event is lost
		for _, testShell := range validShells {
			opt := session.Option()
			opt.Command = []string{testShell}
			if err = Exec(k8sClient, cfg, session); err == nil {
				zaplogger.Sugar().Error(err)
				break
			}
		}
	}
	if err != nil {
		zaplogger.Sugar().Error(err)
		session.Close(err.Error())
		return
	}
	session.Close(ReasonProcessExited)
}

// Exec is called by Terminal
// Executed cmd in the container specified in request and connects it up with the ptyHandler (a Session)
func Exec(k8sClient kubernetes.Interface, cfg *rest.Config, session Session) error {
	zaplogger.Sugar().Infof("startProcess Namespace:%s PodName:%s ContainerName:%s Command:%v",
		session.Option().Namespace, session.Option().PodName, session.Option().ContainerName, session.Option().Command)
	req := k8sClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(session.Option().PodName).
		Namespace(session.Option().Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: session.Option().ContainerName,
			Command:   session.Option().Command,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	zaplogger.Sugar().Infow("Exec", "url", req.URL())

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		zaplogger.Sugar().Error(err)
		return err
	}

	//var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:             session,
		Stdout:            session,
		Stderr:            session,
		TerminalSizeQueue: session,
		Tty:               true,
	})
	if err != nil {
		zaplogger.Sugar().Error(err)
		return err
	}

	return nil
}
