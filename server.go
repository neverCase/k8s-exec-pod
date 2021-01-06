package k8s_exec_pod

import (
	"context"
	"fmt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
	"net/http"
	"time"
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

func InitServer(ctx context.Context, addr, kubeconfig, masterUrl string) *Server {
	cfg, k8sClient := NewResource(masterUrl, kubeconfig)
	h := &Server{
		sessionHub: NewSessionHub(k8sClient, cfg),
	}

	router := gin.New()
	//router.Use(gin.LoggerWithConfig(gin.LoggerConfig{Output: writer}), gin.RecoveryWithWriter(writer))
	router.Use(cors.Default())
	router.GET("/namespace/:namespace/pod/:pod/shell/:container/:command", func(c *gin.Context) {
		option := ExecOptions{
			Namespace:     c.Param("namespace"),
			PodName:       c.Param("pod"),
			ContainerName: c.Param("container"),
			Command:       []string{c.Param("command")},
		}
		var res HttpResponse
		session, err := h.sessionHub.New(option)
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
	router.GET("/log/:token", h.Log)
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
	session, err := s.sessionHub.Get(token)
	if err != nil {
		klog.V(2).Info(err)
		return
	}
	session.HandleSSH(proxy)
}

func (s *Server) Log(c *gin.Context) {
	token := c.Param("token")
	klog.Info("Log token:", token)
	proxy, err := NewProxy(context.Background(), c.Writer, c.Request)
	if err != nil {
		klog.V(2).Info(err)
		return
	}
	session, err := s.sessionHub.Get(token)
	if err != nil {
		klog.V(2).Info(err)
		return
	}
	go session.HandleLog(proxy)
}
