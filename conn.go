package k8s_exec_pod

import (
	"bytes"
	"context"
	"fmt"
	"github.com/Shanghai-Lunara/pkg/zaplogger"
	"github.com/gorilla/websocket"
	"net/http"
	"sync"
	"time"
)

var upGrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024 * 1024 * 10,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Proxy interface {
	ReadPump()
	WritePump()
	Close()
	Recv() (*message, error)
	KeepAlive()
	HandlePing()
	Send(messageType int, data []byte) error
	LoadBuffers(buf []byte) (n int, err error)
	HandleInput(buf []byte, appendBuf []byte) (n int, err error)
}

func NewProxy(ctx context.Context, w http.ResponseWriter, r *http.Request) (Proxy, error) {
	conn, err := upGrader.Upgrade(w, r, nil)
	if err != nil {
		zaplogger.Sugar().Error(err)
		return nil, err
	}
	subCtx, cancel := context.WithCancel(ctx)
	p := &proxy{
		conn:             conn,
		status:           proxyAlive,
		readChan:         make(chan *message, 4096),
		writeChan:        make(chan *message, 4096),
		lastPingTime:     time.Now(),
		keepAliveTimeout: 10,
		ctx:              subCtx,
		cancel:           cancel,
	}
	go p.ReadPump()
	go p.WritePump()
	go p.KeepAlive()
	return p, nil
}

type proxyStatus int

const (
	proxyAlive proxyStatus = iota
	proxyClose proxyStatus = 1
)

type proxy struct {
	conn         *websocket.Conn
	status       proxyStatus
	readChan     chan *message
	writeChan    chan *message
	inputBuffers bytes.Buffer

	lastPingTime     time.Time
	keepAliveTimeout int64
	closeOnce        sync.Once
	ctx              context.Context
	cancel           context.CancelFunc
}

type message struct {
	messageType int
	data        []byte
}

func (p *proxy) KeepAlive() {
	defer p.Close()
	tick := time.NewTicker(time.Second * time.Duration(1+p.keepAliveTimeout))
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if time.Now().Sub(p.lastPingTime) > time.Second*time.Duration(p.keepAliveTimeout) {
				zaplogger.Sugar().Info("Proxy KeepAlive timeout")
				return
			}
		}
	}
}

func (p *proxy) HandlePing() {
	p.lastPingTime = time.Now()
}

func (p *proxy) ReadPump() {
	defer p.Close()
	for {
		messageType, data, err := p.conn.ReadMessage()
		//zaplogger.Sugar().Info("data:", data)
		//zaplogger.Sugar().Infof("messageType: %d message: %v err: %s\n", messageType, data, err)
		if err != nil {
			zaplogger.Sugar().Error(err)
			return
		}
		msg := &message{messageType: messageType, data: data}
		select {
		case p.readChan <- msg:
		case <-p.ctx.Done():
			zaplogger.Sugar().Info("ReadPump: proxy ctx cancel")
			return
		case <-time.After(time.Second * 5):
			zaplogger.Sugar().Info("ReadPump: write into readChan timeout 5s")
			return
		}
	}
}

func (p *proxy) WritePump() {
	defer p.Close()
	for {
		select {
		case msg, isClose := <-p.writeChan:
			if !isClose {
				return
			}
			//zaplogger.Sugar().Info("proxy WritePump msg-data:", string(msg.data))
			if err := p.conn.WriteMessage(msg.messageType, msg.data); err != nil {
				zaplogger.Sugar().Error(err)
				return
			}
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *proxy) Close() {
	zaplogger.Sugar().Info("proxy close")
	p.closeOnce.Do(func() {
		if p.status == proxyClose {
			return
		}
		p.cancel()
		p.status = proxyClose
		//close(p.readChan)
		//close(p.writeChan)
		if err := p.conn.Close(); err != nil {
			zaplogger.Sugar().Error(err)
		}
	})
}

func (p *proxy) Recv() (*message, error) {
	//zaplogger.Sugar().Info("proxy Recv message")
	select {
	case msg, isClose := <-p.readChan:
		if !isClose {
			return nil, fmt.Errorf("readChan closed")
		}
		//zaplogger.Sugar().Info("proxy recv-msg:", msg)
		//zaplogger.Sugar().Info("proxy recv-data-string:", string(msg.data))
		return msg, nil
	case <-p.ctx.Done():
		return nil, fmt.Errorf("proxy ctx cancel")
	}
}

func (p *proxy) Send(messageType int, data []byte) error {
	//zaplogger.Sugar().Infof("proxy send messageType:%v data:%v", messageType, string(data))
	if p.status == proxyClose {
		return fmt.Errorf("err: proxy has been closed")
	}
	select {
	case p.writeChan <- &message{messageType: messageType, data: data}:
		return nil
	case <-p.ctx.Done():
		return fmt.Errorf("proxy ctx cancel")
	}
}

func (p *proxy) LoadBuffers(buf []byte) (n int, err error) {
	if p.inputBuffers.Len() > 0 {
		n = copy(buf, p.inputBuffers.Bytes())
		p.inputBuffers.Next(n)
	}
	return n, nil
}

func (p *proxy) HandleInput(buf []byte, appendBuf []byte) (n int, err error) {
	p.inputBuffers.Write(appendBuf)
	return p.LoadBuffers(buf)
}
