# Copyright Jetstack Ltd. See LICENSE for details.
FROM ubuntu:22.04
LABEL description="OIDC reverse proxy authenticator based on Kubernetes"

ARG TARGETARCH
ARG BIN_NAME=kube-oidc-proxy-${TARGETARCH}

RUN apt-get update && \
    apt-get -y install ca-certificates && \
    apt-get -y upgrade && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY ./bin/${BIN_NAME} /usr/bin/kube-oidc-proxy

CMD ["/usr/bin/kube-oidc-proxy"]
