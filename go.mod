module github.com/nevercase/k8s-exec-pod

go 1.15

require (
	github.com/gin-contrib/cors v1.3.1
	github.com/gin-gonic/gin v1.6.3
	github.com/gorilla/websocket v1.4.2
	github.com/nevercase/k8s-controller-custom-resource v0.0.0-20200909074945-200361074b93
	k8s.io/api v0.17.3
	k8s.io/client-go v0.0.0-20190918160344-1fbdaa4c8d90
	k8s.io/klog/v2 v2.4.0
)
