# This Dockerfile is used to build the image available on DockerHub
FROM centos:centos7 as build

# Add everything
ADD . /usr/src/multus-cni

ENV INSTALL_PKGS "git golang"
RUN rpm --import https://mirror.go-repo.io/centos/RPM-GPG-KEY-GO-REPO && \
    curl -s https://mirror.go-repo.io/centos/go-repo.repo | tee /etc/yum.repos.d/go-repo.repo && \
    yum install -y $INSTALL_PKGS && \
    rpm -V $INSTALL_PKGS && \
    cd /usr/src/multus-cni && \
    ./build

FROM centos:centos7
ADD ./images/cni /usr/src/multus-cni/cni
ADD ./images/package /usr/src/multus-cni/package
COPY --from=build /usr/src/multus-cni/bin /usr/src/multus-cni/bin
WORKDIR /

ADD ./images/entrypoint.sh /
ENTRYPOINT ["/entrypoint.sh"]
