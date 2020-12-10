.PHONY: build mod

HARBOR_DOMAIN := $(shell echo ${HARBOR})
PROJECT := lunara-common
K8S_EXEC_POD_IMAGE := "$(HARBOR_DOMAIN)/$(PROJECT)/k8s-exec-pod:latest"

build:
	-i docker image rm $(K8S_EXEC_POD_IMAGE)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o k8s-exec-pod server/main.go
	cp server/Dockerfile . && docker build -t $(K8S_EXEC_POD_IMAGE) .
	rm -f Dockerfile && rm -f k8s-exec-pod
	docker push $(K8S_EXEC_POD_IMAGE)


mod:
	go mod download
	go mod tidy


