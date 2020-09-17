# k8s-exec-pod
This is a simple and easy way for you to execute commands inside a k8s pod or watch logs  through the websocket proxy.

## build server
```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o exec example/main.go
...
./exec -kubeconfig=$HOME/.kube/config -v 4
```
- if you run the exec binary file inside a k8s pod, just use the command below:
```
./exec -v 4
```

## run websocket_client for testing

### log mode
```sh
go run websocket_client.go --addr=host:port -alsologtostderr=true -v 4 --mode=log
```

### ssh mode
```sh
go run websocket_client.go --addr=host:port -alsologtostderr=true -v 4 --mode=ssh
```

