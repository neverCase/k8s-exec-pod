package k8s_exec_pod

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog"
)

// ExecOptions passed to ExecWithOptions
type ExecOptions struct {
	Command       []string
	Namespace     string
	PodName       string
	ContainerName string
	Stdin         io.Reader
	CaptureStdout bool
	CaptureStderr bool
	// If false, whitespace in std{err,out} will be removed.
	PreserveWhitespace bool
}

const EndOfTransmission = "\u0004"

// TerminalSession implements PtyHandler (using a SockJS connection)
type TerminalSession struct {
	id               string
	bound            chan error
	websocketSession Proxy
	sizeChan         chan remotecommand.TerminalSize
	doneChan         chan struct{}
}

// TerminalMessage is the messaging protocol between ShellController and TerminalSession.
//
// OP      DIRECTION  FIELD(S) USED  DESCRIPTION
// ---------------------------------------------------------------------
// bind    fe->be     SessionID      Id sent back from TerminalResponse
// stdin   fe->be     Data           Keystrokes/paste buffer
// resize  fe->be     Rows, Cols     New terminal size
// stdout  be->fe     Data           Output from the process
// toast   be->fe     Data           OOB message to be shown to the user
type TerminalMessage struct {
	Op, Data, SessionID string
	Rows, Cols          uint16
}

// TerminalSize handles pty->process resize events
// Called in a loop from remotecommand as long as the process is running
func (t TerminalSession) Next() *remotecommand.TerminalSize {
	klog.Info("Next")
	select {
	case size := <-t.sizeChan:
		return &size
	case <-t.doneChan:
		klog.Info("TerminalSession Next doneCHan")
		return nil
	}
}

// Read handles pty->process messages (stdin, resize)
// Called in a loop from remotecommand as long as the process is running
func (t TerminalSession) Read(p []byte) (int, error) {
	klog.Info("TerminalSession Read p:", string(p))
	if n, err := t.websocketSession.LoadBuffers(p); err != nil {
		return 0, err
	} else {
		if n > 0 {
			return n, nil
		}
	}
	var wsMsg *message
	var err error
	if wsMsg, err = t.websocketSession.Recv(); err != nil {
		klog.V(2).Info(err)
		return 0, err
	}

	var msg XtermMsg
	if err := json.Unmarshal(wsMsg.data, &msg); err != nil {
		klog.V(2).Info(err)
		return copy(p, EndOfTransmission), err
	}

	switch msg.MsgType {
	case XtermMsgTypeResize:
		t.sizeChan <- remotecommand.TerminalSize{Width: msg.Cols, Height: msg.Rows}
		return 0, nil
	case XtermMsgTypeInput:
		return t.websocketSession.HandleInput(p, []byte(msg.Input))
	default:
		return copy(p, EndOfTransmission), fmt.Errorf("unknown message type '%s'", msg.MsgType)
	}
}

// Write handles process->pty stdout
// Called from remotecommand whenever there is any output
func (t TerminalSession) Write(p []byte) (int, error) {
	if err := t.websocketSession.Send(websocket.BinaryMessage, p); err != nil {
		klog.V(2).Info(err)
		return 0, err
	}
	return len(p), nil
}

// Toast can be used to send the user any OOB messages
// hterm puts these in the center of the terminal
func (t TerminalSession) Toast(p string) error {
	klog.Info("Toast")
	if err := t.websocketSession.Send(websocket.BinaryMessage, []byte(p)); err != nil {
		klog.V(2).Info(err)
		return err
	}
	return nil
}

// SessionMap stores a map of all TerminalSession objects and a lock to avoid concurrent conflict
type SessionMap struct {
	Sessions map[string]TerminalSession
	Lock     sync.RWMutex
}

// Get return a given terminalSession by sessionId
func (sm *SessionMap) Get(sessionId string) TerminalSession {
	sm.Lock.RLock()
	defer sm.Lock.RUnlock()
	return sm.Sessions[sessionId]
}

// Set store a TerminalSession to SessionMap
func (sm *SessionMap) Set(sessionId string, session TerminalSession) {
	sm.Lock.Lock()
	defer sm.Lock.Unlock()
	sm.Sessions[sessionId] = session
}

// Close shuts down the SockJS connection and sends the status code and reason to the client
// Can happen if the process exits or if there is an error starting up the process
// For now the status code is unused and reason is shown to the user (unless "")
func (sm *SessionMap) Close(sessionId string, status uint32, reason string) {
	sm.Lock.Lock()
	defer sm.Lock.Unlock()
	if _, ok := sm.Sessions[sessionId]; ok {
		sm.Sessions[sessionId].websocketSession.Close()
	}
	delete(sm.Sessions, sessionId)
}

var terminalSessions = SessionMap{Sessions: make(map[string]TerminalSession)}

// handleTerminalSession is Called by net/http for any new /api/sockjs connections
func handleTerminalSession(token string, websocketProxy Proxy) {
	var (
		//buf             string
		//err             error
		//msg             TerminalMessage
		terminalSession TerminalSession
	)
	//if buf, err = session.Recv(); err != nil {
	//	log.Printf("handleTerminalSession: can't Recv: %v", err)
	//	return
	//}
	//
	//if err = json.Unmarshal([]byte(buf), &msg); err != nil {
	//	log.Printf("handleTerminalSession: can't UnMarshal (%v): %s", err, buf)
	//	return
	//}
	//
	//if msg.Op != "bind" {
	//	log.Printf("handleTerminalSession: expected 'bind' message, got: %s", buf)
	//	return
	//}

	if terminalSession = terminalSessions.Get(token); terminalSession.id == "" {
		klog.V(2).Infof("handleTerminalSession: can't find session '%s'", token)
		return
	}

	//todo bug: line 196 send on closed channel
	terminalSession.websocketSession = websocketProxy
	terminalSessions.Set(token, terminalSession)
	terminalSession.bound <- nil
}

// CreateAttachHandler is called from main for /api/sockjs
//func CreateAttachHandler(path string) http.Handler {
//	//return http.HandleFunc("/", s.wsHandler)
//	return sockjs.NewHandler(path, sockjs.DefaultOptions, handleTerminalSession)
//}

// startProcess is called by handleAttach
// Executed cmd in the container specified in request and connects it up with the ptyHandler (a session)
func startProcess(k8sClient kubernetes.Interface, cfg *rest.Config, option ExecOptions, ptyHandler PtyHandler) error {
	klog.Infof("startProcess Namespace:%s PodName:%s ContainerName:%s Command:%v", option.Namespace, option.PodName, option.ContainerName, option.Command)
	req := k8sClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(option.PodName).
		Namespace(option.Namespace).
		SubResource("exec")

	klog.Info("cmd:", option.Command)
	req.VersionedParams(&corev1.PodExecOptions{
		Container: option.ContainerName,
		Command:   option.Command,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}, scheme.ParameterCodec)

	klog.Info("url:", req.URL())

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		klog.V(2).Info(err)
		return err
	}

	//var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:             ptyHandler,
		Stdout:            ptyHandler,
		Stderr:            ptyHandler,
		TerminalSizeQueue: ptyHandler,
		Tty:               true,
	})
	if err != nil {
		klog.V(2).Info(err)
		return err
	}

	//klog.Info(stdout.String(), stderr.String())

	return nil
}

// genTerminalSessionId generates a random session ID string. The format is not really interesting.
// This ID is used to identify the session when the client opens the SockJS connection.
// Not the same as the SockJS session id! We can't use that as that is generated
// on the client side and we don't have it yet at this point.
func genTerminalSessionId() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	id := make([]byte, hex.EncodedLen(len(bytes)))
	hex.Encode(id, bytes)
	return string(id), nil
}

// isValidShell checks if the shell is an allowed one
func isValidShell(validShells []string, shell string) bool {
	klog.Info("isValidShell:", shell)
	for _, validShell := range validShells {
		if validShell == shell {
			return true
		}
	}
	return false
}

// WaitForTerminal is called from apihandler.handleAttach as a goroutine
// Waits for the SockJS connection to be opened by the client the session to be bound in handleTerminalSession
func WaitForTerminal(k8sClient kubernetes.Interface, cfg *rest.Config, option ExecOptions, sessionId string) {
	//shell := request.QueryParameter("shell")

	select {
	case <-terminalSessions.Get(sessionId).bound:
		close(terminalSessions.Get(sessionId).bound)

		var err error
		validShells := []string{"bash", "sh", "powershell", "cmd"}

		if isValidShell(validShells, option.Command[0]) {
			klog.Info(3333333)
			err = startProcess(k8sClient, cfg, option, terminalSessions.Get(sessionId))
		} else {
			klog.Info(44444)
			// No shell given or it was not valid: try some shells until one succeeds or all fail
			// FIXME: if the first shell fails then the first keyboard event is lost
			for _, testShell := range validShells {
				//cmd := []string{testShell}
				option.Command = []string{testShell}
				if err = startProcess(k8sClient, cfg, option, terminalSessions.Get(sessionId)); err == nil {
					break
				}
			}
		}

		if err != nil {
			klog.Info(11111111)
			klog.V(2).Info(err)
			terminalSessions.Close(sessionId, 2, err.Error())
			return
		}
		klog.Info(2222222)
		terminalSessions.Close(sessionId, 1, "Process exited")
	}
}
