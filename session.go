package k8s_exec_pod

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog"
)

// PtyHandler is what remotecommand expects from a pty
type PtyHandler interface {
	io.Reader
	io.Writer
	remotecommand.TerminalSizeQueue
}

// Session implements PtyHandler (using a websocket connection)
type Session interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Next() *remotecommand.TerminalSize
	Id() string
	Wait()
	HandleLog(p Proxy)
	HandleProxy(p Proxy)
	Option() ExecOptions
	Close(reason string)
	Ctx() context.Context
}

const (
	ReasonProcessExited = "process exited"
	ReasonStreamStopped = "stream stopped"
	ReasonConnTimeout   = "conn wait timeout"
	ReasonContextCancel = "ctx cancel"
)

// NewSession returns a new Session Interface
func NewSession(ctx context.Context, connTimeout int64, k8sClient kubernetes.Interface, cfg *rest.Config, option ExecOptions) (Session, error) {
	sessionId, err := genTerminalSessionId()
	if err != nil {
		return nil, err
	}
	subCtx, cancel := context.WithCancel(ctx)
	s := &session{
		sessionId:   sessionId,
		connTimeout: connTimeout,
		option:      option,
		startChan:   make(chan Proxy, 1),
		sizeChan:    make(chan remotecommand.TerminalSize),
		k8sClient:   k8sClient,
		cfg:         cfg,
		context:     subCtx,
		cancel:      cancel,
	}
	go s.Wait()
	return s, nil
}

type session struct {
	sessionId   string
	creatTm     time.Time
	connTimeout int64
	expireTime  time.Time

	option ExecOptions

	sizeChan chan remotecommand.TerminalSize

	startChan      chan Proxy
	websocketProxy Proxy

	k8sClient kubernetes.Interface
	cfg       *rest.Config

	context context.Context
	cancel  context.CancelFunc
	once    sync.Once
}

func (s *session) Id() string {
	return s.sessionId
}

func (s *session) Wait() {
	select {
	case <-time.After(time.Second * time.Duration(s.connTimeout)):
		s.Close(ReasonConnTimeout)
		return
	case proxy := <-s.startChan:
		s.websocketProxy = proxy
		Terminal(s.k8sClient, s.cfg, s)
	case <-s.context.Done():
		return
	}
}

func (s *session) HandleLog(p Proxy) {
	s.websocketProxy = p
	if err := LogTransmit(s.k8sClient, s); err != nil {
		klog.V(2).Info()
	}
}

func (s *session) HandleProxy(p Proxy) {
	select {
	case s.startChan <- p:
		return
	case <-time.After(time.Second * 1):
		return
	}
}

const EndOfTransmission = "\u0004"

func (s *session) Read(p []byte) (int, error) {
	klog.Info("TerminalSession Read p:", string(p))
	if n, err := s.websocketProxy.LoadBuffers(p); err != nil {
		return 0, err
	} else {
		if n > 0 {
			return n, nil
		}
	}
	var wsMsg *message
	var err error
	if wsMsg, err = s.websocketProxy.Recv(); err != nil {
		klog.V(2).Info(err)
		return 0, err
	}

	var msg TermMsg
	if err := json.Unmarshal(wsMsg.data, &msg); err != nil {
		klog.V(2).Info(err)
		return copy(p, EndOfTransmission), err
	}

	switch msg.MsgType {
	case XtermMsgTypeResize:
		s.sizeChan <- remotecommand.TerminalSize{Width: msg.Cols, Height: msg.Rows}
		return 0, nil
	case XtermMsgTypeInput:
		return s.websocketProxy.HandleInput(p, []byte(msg.Input))
	default:
		return copy(p, EndOfTransmission), fmt.Errorf("unknown message type '%s'", msg.MsgType)
	}
}

func (s *session) Write(p []byte) (int, error) {
	klog.Info("session Write:", string(p))
	if err := s.websocketProxy.Send(websocket.BinaryMessage, p); err != nil {
		klog.V(2).Info(err)
		return copy(p, EndOfTransmission), err
	}
	return len(p), nil
}

func (s *session) Next() *remotecommand.TerminalSize {
	select {
	case size := <-s.sizeChan:
		return &size
	case <-s.context.Done():
		return nil
	}
}

func (s *session) Option() ExecOptions {
	return s.option
}

func (s *session) Close(reason string) {
	s.once.Do(func() {
		klog.Infof("sessionId:%s close reason:%s", s.Id(), reason)
		s.cancel()
	})
}

func (s *session) Ctx() context.Context {
	return s.context
}

// genTerminalSessionId generates a random session ID string. The format is not really interesting.
// This ID is used to identify the session when the client opens the Websocket connection.
// Not the same as the Websocket session id! We can't use that as that is generated
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
