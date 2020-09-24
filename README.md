# k8s-exec-pod
This is a simple and easy way for you to execute commands inside a k8s pod or watch logs  through the websocket proxy.

## Notice
- A terminal is not just an input field. 
- It's a complex system that provides advanced formatting and interactivity with the user, over a plain character stream.
- Here is a classic case: if you transmit the command `clear` to the ssh pty, then the response you received would be "clear: command not found"
- So we should listen and compare `each character` instead of inputting a full command with the `\n` (e.g. `"pwd\n"`)

## build server
```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o exec-bin example/main.go
...
./exec-bin  -kubeconfig=$HOME/.kube/config --proxyservice=0.0.0.0:9090 -v 4
```
- if you run the exec binary file inside a k8s pod, just use the command below:
```
./exec-bin --proxyservice=0.0.0.0:9090 -v 4
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

