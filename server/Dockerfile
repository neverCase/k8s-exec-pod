FROM ubuntu:18.04

LABEL maintainer="1024769485@qq.com"

WORKDIR /data/
COPY k8s-exec-pod /data/

CMD ["/data/k8s-exec-pod", "-alsologtostderr=true", "--apiservice=0.0.0.0:9090", "-v", "4"]
