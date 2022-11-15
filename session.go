package k8s_exec_pod

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Shanghai-Lunara/pkg/zaplogger"
	"github.com/gorilla/websocket"
	"io"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sync"
	"time"
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
	HandleSSH(p Proxy)
	Option() *ExecOptions
	Close(reason string)
	Ctx() context.Context
	ReadCloser(rc io.ReadCloser)
}

const (
	ReasonProcessExited = "process exited"
	ReasonStreamStopped = "stream stopped"
	ReasonConnTimeout   = "conn wait timeout"
	ReasonContextCancel = "ctx cancel"
)

// NewSession returns a new Session Interface
func NewSession(ctx context.Context, connTimeout int64, k8sClient kubernetes.Interface, cfg *rest.Config, option *ExecOptions) (Session, error) {
	sessionId, err := genTerminalSessionId()
	if err != nil {
		return nil, err
	}
	subCtx, cancel := context.WithCancel(ctx)
	s := &session{
		sessionId:   sessionId,
		connTimeout: connTimeout,
		option:      option,
		startChan:   make(chan proxyChan, 1),
		sizeChan:    make(chan remotecommand.TerminalSize),
		k8sClient:   k8sClient,
		cfg:         cfg,
		context:     subCtx,
		cancel:      cancel,
	}
	go s.Wait()
	return s, nil
}

type handleType string

const (
	handleSSH handleType = "ssh"
	handleLog handleType = "log"
)

type proxyChan struct {
	t handleType
	p Proxy
}

type session struct {
	sessionId   string
	creatTm     time.Time
	connTimeout int64
	expireTime  time.Time

	option *ExecOptions

	sizeChan chan remotecommand.TerminalSize

	readCloser io.ReadCloser

	startChan      chan proxyChan
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
	case proxyChan := <-s.startChan:
		s.websocketProxy = proxyChan.p
		switch proxyChan.t {
		case handleSSH:
			Terminal(s.k8sClient, s.cfg, s)
		case handleLog:
			go func() {
				for {
					var buf []byte
					if _, err := s.Read(buf); err != nil {
						zaplogger.Sugar().Error(err)
						if s.readCloser != nil {
							zaplogger.Sugar().Info("readCloser was not set")
							return
						}
						go func() {
							defer func() {
								if r := recover(); r != nil {
									zaplogger.Sugar().Info("s.readCloser.Close Recovered in: ", r)
								}
							}()
							if err = s.readCloser.Close(); err != nil {
								zaplogger.Sugar().Error(err)
							}
						}()
						return
					}
				}
			}()
			if err := LogTransmit(s.k8sClient, s); err != nil {
				zaplogger.Sugar().Error(err)
			}
		}
	case <-s.context.Done():
		return
	}
}

func (s *session) HandleLog(p Proxy) {
	select {
	case s.startChan <- proxyChan{t: handleLog, p: p}:
		return
	case <-time.After(time.Second * 1):
		return
	}
}

func (s *session) HandleSSH(p Proxy) {
	select {
	case s.startChan <- proxyChan{t: handleSSH, p: p}:
		return
	case <-time.After(time.Second * 1):
		return
	}
}

const EndOfTransmission = "\u0004"

// Read handles pty->process messages (stdin, resize)
// Called in a loop from remotecommand as long as the process is running
func (s *session) Read(p []byte) (int, error) {
	//zaplogger.Sugar().Infow("TerminalSession", "Read", string(p))
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
		zaplogger.Sugar().Error(err)
		return copy(p, EndOfTransmission), err
	}

	var msg TermMsg
	if err := json.Unmarshal(wsMsg.data, &msg); err != nil {
		zaplogger.Sugar().Error(err)
		return copy(p, EndOfTransmission), err
	}

	switch msg.MsgType {
	case TermResize:
		s.sizeChan <- remotecommand.TerminalSize{Width: msg.Cols, Height: msg.Rows}
		return 0, nil
	case TermInput:
		return s.websocketProxy.HandleInput(p, []byte(msg.Input))
	case TermPing:
		s.websocketProxy.HandlePing()
		return 0, nil
	default:
		return copy(p, EndOfTransmission), fmt.Errorf("unknown message type '%s'", msg.MsgType)
	}
}

// Write handles process->pty stdout
// Called from remotecommand whenever there is any output
// If the TermMsg.MsgType was TermPing, then it would handle Proxy.HandlePing
func (s *session) Write(p []byte) (int, error) {
	//zaplogger.Sugar().Infow("TerminalSession", "Write", string(p))
	data := make([]byte, len(p))
	copy(data, p)
	if err := s.websocketProxy.Send(websocket.BinaryMessage, data); err != nil {
		zaplogger.Sugar().Error(err)
		return 0, err
	}
	return len(p), nil
}

// Next handles pty->process resize events
// Called in a loop from remotecommand as long as the process is running
func (s *session) Next() *remotecommand.TerminalSize {
	select {
	case size := <-s.sizeChan:
		return &size
	case <-s.context.Done():
		return nil
	}
}

func (s *session) Option() *ExecOptions {
	return s.option
}

func (s *session) Close(reason string) {
	zaplogger.Sugar().Infow("TerminalSession trigger close", "sessionId", s.Id(), "reason", reason)
	s.once.Do(func() {
		zaplogger.Sugar().Infow("TerminalSession successfully close", "sessionId", s.Id(), "reason", reason)
		s.cancel()
	})
}

func (s *session) Ctx() context.Context {
	return s.context
}

func (s *session) ReadCloser(rc io.ReadCloser) {
	s.readCloser = rc
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
