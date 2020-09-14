package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/nevercase/k8s-controller-custom-resource/pkg/signals"
	"io/ioutil"
	"k8s.io/klog"
	"log"
	"net/http"
	"net/url"
	"time"

	exec "github.com/nevercase/k8s-exec-pod"
)

var (
	addr string
)

func init() {
	flag.StringVar(&addr, "addr", "", "ws addr")
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()
	s := &Service{ctx: context.Background()}
	token, err := getToken(addr)
	if err != nil {
		klog.Fatal(err)
	}
	//klog.Fatal(token)
	c, err := s.newClient(addr, token)
	if err != nil {
		log.Panic(err)
	}
	s.c = c
	//c.writeChan <- exec.XtermMsg{MsgType: "input", Input: "pwd"}
	c.writeChan <- exec.XtermMsg{MsgType: "resize", Rows: 100, Cols: 100}
	time.Sleep(time.Second * 2)
	c.writeChan <- exec.XtermMsg{MsgType: "input", Input: "pwd\n"}
	//c.writeChan <- exec.XtermMsg{MsgType: "resize", Rows: 100, Cols: 100}
	time.Sleep(time.Second * 2)
	c.writeChan <- exec.XtermMsg{MsgType: "input", Input: "ls -al\n"}
	//c.writeChan <- exec.XtermMsg{MsgType: "resize", Rows: 100, Cols: 100}
	//for {
	//	c.writeChan <- exec.XtermMsg{MsgType: "input", Input: "ls -al"}
	//	time.Sleep(time.Second * 5)
	//	//break
	//}
	<-stopCh
}

func getToken(addr string) (string, error) {
	requestUrl := fmt.Sprintf("http://%s/namespace/develop/pod/hso-develop-campaign-0/shell/hso-develop-campaign/bash", addr)
	res, err := http.Get(requestUrl)
	if err != nil {
		klog.V(2).Infof("http err:", err)
		return "", err
	}
	cot, err := ioutil.ReadAll(res.Body)
	defer func() {
		if err := res.Body.Close(); err != nil {
			klog.V(2).Infof("http err:", err)
			klog.Info(err)
		}
	}()
	if err != nil {
		klog.V(2).Infof("http err:", err)
		return "", err
	}
	var result exec.HttpResponse
	if err := json.Unmarshal(cot, &result); err != nil {
		klog.V(2).Infof("http err:", err)
		return "", err
	}
	klog.Info(requestUrl)
	klog.Info("token:", result.Token)
	return result.Token, nil
}

type Service struct {
	ctx    context.Context
	cancel context.CancelFunc
	c      *Client
}

type Client struct {
	ws        *websocket.Conn
	writeChan chan interface{}
	ctx       context.Context
	cancel    context.CancelFunc
}

func (s *Service) newClient(addr, token string) (c *Client, err error) {
	var (
		ws *websocket.Conn
	)
	if ws, err = s.conn(addr, token); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c = &Client{
		ws:        ws,
		writeChan: make(chan interface{}),
		ctx:       ctx,
		cancel:    cancel,
	}
	go func() {
		if err = s.readPump(c); err != nil {
			klog.V(2).Info(err)
		}
	}()
	go func() {
		if err = s.writePump(c); err != nil {
			log.Println(err)
		}
	}()
	return c, nil
}

func (s *Service) conn(addr, token string) (ws *websocket.Conn, err error) {
	u := url.URL{Scheme: "ws", Host: addr, Path: fmt.Sprintf("/ssh/%s", token)}
	klog.Info("url:", u)
	a, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		klog.V(2).Info(err)
		return nil, err
	}
	return a, nil
}

func (s *Service) readPump(c *Client) (err error) {
	for {
		messageType, message, err := c.ws.ReadMessage()
		klog.Infof("messageType: %d message: %s  err: %s\n", messageType, string(message), err)
		if err != nil {
			return nil
		}
	}
}

func (s *Service) writePump(c *Client) (err error) {
	for {
		select {
		case <-c.ctx.Done():
			klog.Info("ctx donw")
			return
		case msg := <-c.writeChan:
			klog.Info("writePump get msg:", msg)
			if err = c.ws.WriteJSON(msg); err != nil {
				return err
			}
		}
	}
}
