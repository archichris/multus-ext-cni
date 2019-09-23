# This Dockerfile is used to build the image available on DockerHub
FROM centos:centos7 as build

# Add everything
ADD . /usr/src/multus-cni 

ENV INSTALL_PKGS "git golang"
ARG http_proxy "http://192.168.56.1:808"
ARG https_proxy "http://192.168.56.1:808"
ENV GOPROXY "http://proxy.golang.org,direct"
# RUN echo $http_proxy && echo $https_proxy
RUN rpm --import https://mirror.go-repo.io/centos/RPM-GPG-KEY-GO-REPO && \
    echo "proxy=${http_proxy}" >> /etc/yum.conf && \
    curl -x ${http_proxy} -k -s https://mirror.go-repo.io/centos/go-repo.repo | tee /etc/yum.repos.d/go-repo.repo && \
    yum install -y $INSTALL_PKGS && \
    rpm -V $INSTALL_PKGS && \
    git config --global http.sslVerify false && \
    cd /usr/src/multus-cni && \
    ./build

FROM centos:centos7
ADD ./cni /usr/src/multus-cni/cni
COPY --from=build /usr/src/multus-cni /usr/src/multus-cni
WORKDIR /

ADD ./images/entrypoint.sh /
ENTRYPOINT ["/entrypoint.sh"]
