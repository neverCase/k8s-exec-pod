package k8s_exec_pod

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/klog"
)

const (
	CodeSuccess = iota
	CodeError
)

//var json = jsoniter.ConfigCompatibleWithStandardLibrary

type Server struct {
	server *http.Server
	ctx    context.Context

	sessionHub SessionHub
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

func InitServer(ctx context.Context, addr, kubeconfig, masterUrl string) *Server {
	cfg, k8sClient := NewResource(masterUrl, kubeconfig)
	h := &Server{
		sessionHub: NewSessionHub(k8sClient, cfg),
	}

	router := gin.New()
	//router.Use(gin.LoggerWithConfig(gin.LoggerConfig{Output: writer}), gin.RecoveryWithWriter(writer))
	router.Use(header())
	router.GET("/namespace/:namespace/pod/:pod/shell/:container/:command", func(c *gin.Context) {
		option := ExecOptions{
			Namespace:     c.Param("namespace"),
			PodName:       c.Param("pod"),
			ContainerName: c.Param("container"),
			Command:       []string{c.Param("command")},
		}
		var res HttpResponse
		session, err := h.sessionHub.NewSession(option)
		if err != nil {
			res.Code = CodeError
			res.Message = fmt.Sprintf("Failed to init session err:%s", err.Error())
		} else {
			res.Code = CodeSuccess
			res.Token = session.Id()
		}
		klog.Infof("Namespace:%s PodName:%s ContainerName:%s Command:%v", option.Namespace, option.PodName, option.ContainerName, option.Command)
		c.JSON(http.StatusOK, res)
	})
	router.GET("/ssh/:token", h.SSH)
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

func (s *Server) ShutDown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		klog.V(2).Infof("http.Server shutdown err:", err)
	}
}

func (s *Server) SSH(c *gin.Context) {
	token := c.Param("token")
	klog.Info("SSH token:", token)
	proxy, err := NewProxy(context.Background(), c.Writer, c.Request)
	if err != nil {
		klog.V(2).Info(err)
		return
	}
	session, err := s.sessionHub.Session(token)
	if err != nil {
		klog.V(2).Info(err)
		return
	}
	session.Start(proxy)
}
