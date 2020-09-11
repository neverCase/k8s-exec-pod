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
	s := &Service{}
	token, err := getToken(addr)
	if err != nil {
		klog.Fatal(err)
	}
	c, err := s.newClient(addr, token)
	if err != nil {
		log.Panic(err)
	}
	_ = c
	<-stopCh
}

func getToken(addr string) (string, error) {
	url := fmt.Sprintf("http://%s/pod/develop/hso-develop-campaign-0/shell/hso-develop-campaign/bash", addr)
	res, err := http.Get(url)
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
	var resoult exec.HttpResponse
	if err := json.Unmarshal(cot, &resoult); err != nil {
		klog.V(2).Infof("http err:", err)
		return "", err
	}
	klog.Info(url)
	klog.Info("token:", resoult.Token)
	return resoult.Token, nil
}

type Service struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type Client struct {
	ws        *websocket.Conn
	teams     map[string]string
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
	ctx, cancel := context.WithCancel(s.ctx)
	c = &Client{
		ws:        ws,
		writeChan: make(chan interface{}),
		ctx:       ctx,
		cancel:    cancel,
	}
	go func() {
		if err = s.readPump(c); err != nil {
			log.Println(err)
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
	log.Println("url:", u)
	a, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Service) readPump(c *Client) (err error) {
	for {
		messageType, message, err := c.ws.ReadMessage()
		log.Printf("messageType: %d message: %s  err: %s\n", messageType, string(message), err)
		if err != nil {
			return nil
		}
	}
}

func (s *Service) writePump(c *Client) (err error) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case msg := <-c.writeChan:
			if err = c.ws.WriteJSON(msg); err != nil {
				return err
			}
		}
	}
}
