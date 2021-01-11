FROM ubuntu:18.04

LABEL maintainer="1024769485@qq.com"

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install tzdata

WORKDIR /k8s-api/
COPY k8s-exec-pod /k8s-api/

ENV TZ=Asia/Shanghai
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone
RUN dpkg-reconfigure -f noninteractive tzdata

CMD ["/k8s-api/k8s-exec-pod", "-alsologtostderr=true", "--apiservice=0.0.0.0:9090", "-v", "4"]
