# Copyright Jetstack Ltd. See LICENSE for details.
FROM ubuntu:22.04
LABEL description="OIDC reverse proxy authenticator based on Kubernetes"

ARG TARGETARCH

RUN apt-get update && \
    apt-get -y install ca-certificates && \
    apt-get -y upgrade && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Use platform-specific binary built externally (from Makefile)
COPY ./bin/kube-oidc-proxy-${TARGETARCH} /usr/bin/kube-oidc-proxy

CMD ["/usr/bin/kube-oidc-proxy"]
