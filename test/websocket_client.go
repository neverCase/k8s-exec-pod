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
	mode string
)

func init() {
	flag.StringVar(&addr, "addr", "", "ws addr")
	flag.StringVar(&mode, "mode", "ssh", "mode")
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
	klog.Info(token)
	var c *Client
	switch mode {
	case "ssh":
		//<-time.After(time.Second * 21)
		klog.Info("sleep 31 seconds")
		c, err = s.newClient(addr, "ssh", token)
		if err != nil {
			log.Panic(err)
		}
		go ping(c)
		ssh(c)
	case "log":
		c, err = s.newClient(addr, "log", token)
		if err != nil {
			log.Panic(err)
		}
		go ping(c)
	}
	<-stopCh
}

func ssh(c *Client) {
	//c.writeChan <- exec.TermMsg{MsgType: "input", Input: "pwd"}
	c.writeChan <- exec.TermMsg{MsgType: "resize", Rows: 110, Cols: 54}
	time.Sleep(time.Second * 2)
	// clear: command not found
	c.writeChan <- exec.TermMsg{MsgType: "input", Input: "clear\n"}
	//c.writeChan <- exec.TermMsg{MsgType: "input", Input: "pwd"}
	c.writeChan <- exec.TermMsg{MsgType: "input", Input: "p"}
	c.writeChan <- exec.TermMsg{MsgType: "input", Input: "w"}
	c.writeChan <- exec.TermMsg{MsgType: "input", Input: "d"}
	c.writeChan <- exec.TermMsg{MsgType: "input", Input: "\n"}
	//c.writeChan <- exec.TermMsg{MsgType: "resize", Rows: 100, Cols: 100}
	time.Sleep(time.Second * 2)
	c.writeChan <- exec.TermMsg{MsgType: "input", Input: "l"}
	c.writeChan <- exec.TermMsg{MsgType: "input", Input: "s"}
	c.writeChan <- exec.TermMsg{MsgType: "input", Input: " "}
	c.writeChan <- exec.TermMsg{MsgType: "input", Input: "-"}
	c.writeChan <- exec.TermMsg{MsgType: "input", Input: "a"}
	c.writeChan <- exec.TermMsg{MsgType: "input", Input: "l"}
	c.writeChan <- exec.TermMsg{MsgType: "input", Input: "\n"}
	//c.writeChan <- exec.TermMsg{MsgType: "input", Input: "\n"}
	//c.writeChan <- exec.TermMsg{MsgType: "resize", Rows: 100, Cols: 100}
	time.Sleep(time.Second * 2)
	//c.writeChan <- exec.TermMsg{MsgType: "input", Input: "top\n"}
}

func ping(c *Client) {
	for {
		c.writeChan <- exec.TermMsg{MsgType: "ping"}
		time.Sleep(time.Second * 5)
	}
}

func getToken(addr string) (string, error) {
	requestUrl := fmt.Sprintf("http://%s/namespace/develop/pod/hso-develop-campaign-0/shell/hso-develop-campaign/bash", addr)
	//requestUrl := fmt.Sprintf("http://%s/namespace/kube-system/pod/traefik-8454d5446b-jdzwl/shell/traefik/bash", addr)
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

func (s *Service) newClient(addr, mode, token string) (c *Client, err error) {
	var (
		ws *websocket.Conn
	)
	if ws, err = s.conn(addr, mode, token); err != nil {
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

func (s *Service) conn(addr, mode, token string) (ws *websocket.Conn, err error) {
	u := url.URL{Scheme: "ws", Host: addr, Path: fmt.Sprintf("/%s/%s", mode, token)}
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
