package k8s_exec_pod

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog"
)

const (
	CodeSuccess = iota
	CodeError
)

//var json = jsoniter.ConfigCompatibleWithStandardLibrary

type Http struct {
	server *http.Server
	ctx    context.Context

	k8sClient kubernetes.Interface
	cfg       *rest.Config
}

func header() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")
		var headerKeys []string
		for k, v := range c.Request.Header {
			_ = v
			headerKeys = append(headerKeys, k)
		}
		headerStr := strings.Join(headerKeys, ", ")
		if headerStr != "" {
			headerStr = fmt.Sprintf("access-control-allow-origin, access-control-allow-headers, %s", headerStr)
		} else {
			headerStr = "access-control-allow-origin, access-control-allow-headers"
		}
		if origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE,UPDATE")
			//  header types
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Length, X-CSRF-Token, Token,session,X_Requested_With,Accept, Origin, Host, Connection, Accept-Encoding, Accept-Language,DNT, X-CustomHeader, Keep-Alive, User-Agent, X-Requested-With, If-Modified-Since, Cache-Control, Content-Type, Pragma")
			c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers,Cache-Control,Content-Language,Content-Type,Expires,Last-Modified,Pragma,FooBar")
			c.Header("Access-Control-Max-Age", "172800")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Set("content-type", "application/json")
		}
		if method == "OPTIONS" {
			c.JSON(http.StatusOK, "Options Request!")
		}
		c.Next()
	}
}

func InitServer(ctx context.Context, addr, kubeconfig, masterUrl string) *Http {
	h := &Http{}
	h.cfg, h.k8sClient = NewResource(masterUrl, kubeconfig)
	router := gin.New()
	//router.Use(gin.LoggerWithConfig(gin.LoggerConfig{Output: writer}), gin.RecoveryWithWriter(writer))
	router.Use(header())
	router.GET("/namespace/:namespace/pod/:pod/shell/:container/:command", func(c *gin.Context) {
		var res HttpResponse
		session, err := genTerminalSessionId()
		if err != nil {
			res.Code = CodeError
			res.Message = "Failed to get genTerminalSessionId"
		} else {
			res.Code = CodeSuccess
			res.Token = session
		}
		terminalSessions.Set(session, TerminalSession{
			id:       session,
			bound:    make(chan error),
			sizeChan: make(chan remotecommand.TerminalSize),
		})
		option := ExecOptions{
			Namespace:     c.Param("namespace"),
			PodName:       c.Param("pod"),
			ContainerName: c.Param("container"),
			Command:       []string{c.Param("command")},
		}
		klog.Infof("Namespace:%s PodName:%s ContainerName:%s Command:%v", option.Namespace, option.PodName, option.ContainerName, option.Command)
		go WaitForTerminal(h.k8sClient, h.cfg, option, session)
		c.JSON(http.StatusOK, res)
	})
	router.GET("/ssh/:token", SSH)
	h.server = &http.Server{
		Addr:    addr,
		Handler: router,
	}
	go func() {
		if err := h.server.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				klog.Info("Server closed under request")
			} else {
				klog.V(2).Info("Server closed unexpected err:", err)
			}
		}
	}()
	return h
}

func (h *Http) ShutDown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.server.Shutdown(ctx); err != nil {
		klog.V(2).Infof("http.Server shutdown err:", err)
	}
}

func SSH(c *gin.Context) {
	token := c.Param("token")
	klog.Info("SSH token:", token)
	proxy, err := NewProxy(context.Background(), c.Writer, c.Request)
	if err != nil {
		klog.V(2).Info(err)
		return
	}
	handleTerminalSession(token, proxy)
}
