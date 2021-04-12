package k8s_exec_pod

import (
	"context"
	"fmt"
	"github.com/Shanghai-Lunara/pkg/zaplogger"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"io"
	"k8s.io/client-go/kubernetes"
	"net/http"
	"strconv"
	"time"
)

const (
	CodeSuccess = iota
	CodeError
)

type Server struct {
	server     *http.Server
	ctx        context.Context
	k8sClient  kubernetes.Interface
	sessionHub SessionHub
}

func InitServer(ctx context.Context, addr, kubeconfig, masterUrl string) *Server {
	cfg, k8sClient := NewResource(masterUrl, kubeconfig)
	h := &Server{
		k8sClient:  k8sClient,
		sessionHub: NewSessionHub(k8sClient, cfg),
	}
	router := gin.New()
	router.Use(cors.Default())
	router.GET(RouterPodShellToken, h.PodToken)
	router.GET(RouterSSH, h.SSH)
	router.GET(RouterLog, h.LogStream)
	router.GET(RouterPodLogDownload, h.LogDownload)
	h.server = &http.Server{
		Addr:    addr,
		Handler: router,
	}
	go func() {
		if err := h.server.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				zaplogger.Sugar().Info("Server closed under request")
			} else {
				zaplogger.Sugar().Info("Server closed unexpected err:", err)
			}
		}
	}()
	return h
}

func (s *Server) ShutDown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		zaplogger.Sugar().Errorf("http.Server shutdown err:%v", err)
	}
}

func (s *Server) PodToken(c *gin.Context) {
	option := &ExecOptions{
		Namespace:     c.Param("namespace"),
		PodName:       c.Param("pod"),
		ContainerName: c.Param("container"),
		Follow:        true,
		Command:       []string{c.Param("command")},
	}
	var res HttpResponse
	session, err := s.sessionHub.New(option)
	if err != nil {
		res.Code = CodeError
		res.Message = fmt.Sprintf("Failed to init session err:%s", err.Error())
	} else {
		res.Code = CodeSuccess
		res.Token = session.Id()
	}
	zaplogger.Sugar().Infof("Namespace:%s PodName:%s ContainerName:%s Command:%v", option.Namespace, option.PodName, option.ContainerName, option.Command)
	c.JSON(http.StatusOK, res)
}

func (s *Server) SSH(c *gin.Context) {
	token := c.Param("token")
	zaplogger.Sugar().Info("SSH token:", token)
	proxy, err := NewProxy(context.Background(), c.Writer, c.Request)
	if err != nil {
		zaplogger.Sugar().Error(err)
		return
	}
	session, err := s.sessionHub.Get(token)
	if err != nil {
		zaplogger.Sugar().Error(err)
		return
	}
	session.HandleSSH(proxy)
}

func (s *Server) LogStream(c *gin.Context) {
	token := c.Param("token")
	zaplogger.Sugar().Info("Log token:", token)
	proxy, err := NewProxy(context.Background(), c.Writer, c.Request)
	if err != nil {
		zaplogger.Sugar().Error(err)
		return
	}
	session, err := s.sessionHub.Get(token)
	if err != nil {
		zaplogger.Sugar().Error(err)
		return
	}
	go session.HandleLog(proxy)
}

//RouterPodLogDownload = "/namespace/:namespace/pod/:pod/:container/previous/:previous/SinceSeconds/:SinceSeconds/SinceTime/:SinceTime"

func (s *Server) LogDownload(c *gin.Context) {
	pre, err := strconv.ParseBool(c.Param("previous"))
	if err != nil {
		zaplogger.Sugar().Error(err)
		c.Abort()
		return
	}
	option := &ExecOptions{
		Namespace:       c.Param("namespace"),
		PodName:         c.Param("pod"),
		ContainerName:   c.Param("container"),
		Follow:          false,
		UsePreviousLogs: pre,
	}
	// check SinceSeconds and SinceTime
	sinceSec, err := strconv.Atoi(c.Param("SinceSeconds"))
	if err != nil {
		zaplogger.Sugar().Errorw("Convert SinceSeconds failed", "SinceSeconds", c.Param("SinceSeconds"), "err", err)
		c.Abort()
		return
	}
	if sinceSec > 0 {
		a := int64(sinceSec)
		option.SinceSeconds = &a
		option.SinceTime = nil
	} else {
	}
	reader, err := LogDownload(s.k8sClient, option)
	if err != nil {
		zaplogger.Sugar().Error(err)
		c.Abort()
		return
	}
	defer func() {
		zaplogger.Sugar().Info("LogTransmit readCloser close")
		if err := reader.Close(); err != nil {
			zaplogger.Sugar().Error(err)
		}
	}()
	fileContentDisposition := fmt.Sprintf("attachment;filename=%s_%s_%s.log", c.Param("namespace"), c.Param("pod"), c.Param("container"))
	c.Header("Content-Type", "text/plain")
	c.Header("Content-Disposition", fileContentDisposition)
	if _, err = io.Copy(c.Writer, reader); err != nil {
		zaplogger.Sugar().Error(err)
	}
}
