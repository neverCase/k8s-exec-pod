package k8s_exec_pod

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
)

const (
	ErrSessionIdNotExist = "error: the session:%v was not exist"
)

type SessionHub interface {
	New(option ExecOptions) (s Session, err error)
	Get(sessionId string) (s Session, err error)
	Close(sessionId string, reason string) error
	Listen(session Session) error
}

func NewSessionHub(k8sClient kubernetes.Interface, cfg *rest.Config) SessionHub {
	return &sessionHub{
		items:     make(map[string]Session, 0),
		k8sClient: k8sClient,
		cfg:       cfg,
	}
}

type sessionHub struct {
	mu    sync.RWMutex
	items map[string]Session

	k8sClient kubernetes.Interface
	cfg       *rest.Config
}

func (sh *sessionHub) New(option ExecOptions) (s Session, err error) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	s, err = NewSession(context.Background(), 10, sh.k8sClient, sh.cfg, option)
	if err != nil {
		return nil, err
	}
	sh.items[s.Id()] = s
	go func() {
		if err := sh.Listen(s); err != nil {
			klog.V(2).Info(err)
		}
	}()
	return s, nil
}

func (sh *sessionHub) Get(sessionId string) (Session, error) {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	if t, ok := sh.items[sessionId]; ok {
		return t, nil
	}
	return nil, fmt.Errorf(ErrSessionIdNotExist, sessionId)
}

func (sh *sessionHub) Close(sessionId string, reason string) error {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if t, ok := sh.items[sessionId]; ok {
		t.Close(reason)
		delete(sh.items, sessionId)
	} else {
		return fmt.Errorf(ErrSessionIdNotExist, sessionId)
	}
	return nil
}

func (sh *sessionHub) Listen(session Session) error {
	select {
	case <-session.Context().Done():
		if err := sh.Close(session.Id(), ReasonContextCancel); err != nil {
			return err
		}
	}
	return nil
}
