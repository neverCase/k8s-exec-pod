package k8s_exec_pod

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	ErrSessionIdNotExist = "error: the session:%v was not exist"
)

type SessionHub interface {
	NewSession(option ExecOptions) (s Session, err error)
	Session(sessionId string) (s Session, err error)
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

func (sh *sessionHub) Session(sessionId string) (Session, error) {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	if t, ok := sh.items[sessionId]; ok {
		return t, nil
	}
	return nil, fmt.Errorf(ErrSessionIdNotExist, sessionId)
}

func (sh *sessionHub) NewSession(option ExecOptions) (s Session, err error) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	s, err = NewSession(context.Background(), 30, sh.k8sClient, sh.cfg, option)
	if err != nil {
		return nil, err
	}
	sh.items[s.Id()] = s
	return s, nil
}
