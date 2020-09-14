package k8s_exec_pod

import (
	"context"
	"io"
	"sync"
	"time"

	"k8s.io/client-go/tools/remotecommand"
)

// PtyHandler is what remotecommand expects from a pty
type PtyHandler interface {
	io.Reader
	io.Writer
	remotecommand.TerminalSizeQueue
}

// Session implements PtyHandler (using a Xtermjs connection)
type Session interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Next() *remotecommand.TerminalSize
	Wait()
}

// NewSession returns a new Session Interface
func NewSession(ctx context.Context, connTimeout int64) Session {
	subCtx, cancel := context.WithCancel(ctx)
	s := &session{
		connTimeout: connTimeout,
		startChan:   make(chan Proxy, 1),
		sizeChan:    make(chan remotecommand.TerminalSize),
		context:     subCtx,
		cancel:      cancel,
	}
	go s.Wait()
	return s
}

type session struct {
	creatTm     time.Time
	connTimeout int64
	expireTime  time.Time

	sizeChan chan remotecommand.TerminalSize

	startChan      chan Proxy
	websocketProxy Proxy

	context context.Context
	cancel  context.CancelFunc
	once    sync.Once
}

func (s *session) Wait() {
	select {
	case <-time.After(time.Second * time.Duration(s.connTimeout)):
		return
	case proxy := <-s.startChan:
		s.websocketProxy = proxy
	case <-s.context.Done():
		return
	}
}

func (s *session) Read(p []byte) (int, error) {

}

func (s *session) Write(p []byte) (int, error) {

}

func (s *session) Next() *remotecommand.TerminalSize {
	select {
	case size := <-s.sizeChan:
		return &size
	case <-s.context.Done():
		return nil
	}
}
