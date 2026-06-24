# One Dockerfile built into two images (control plane and worker node) — see build.sh.

# ---- build stage: compile executor/microtunnel/tundial (native arm64) ----
FROM public.ecr.aws/docker/library/golang:1.26.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
ENV CGO_ENABLED=0
COPY . .
RUN go build -o /out/executor ./executor \
 && go build -o /out/microtunnel ./microtunnel \
 && go build -o /out/tundial  ./tundial

# ---- final stage: k3s node image ----
FROM public.ecr.aws/lambda/microvms:al2023-minimal
RUN dnf install -y \
    bash \
    iproute \
    util-linux \
    procps-ng \
    ethtool \
    iptables \
    iptables-nft \
    conntrack-tools \
    tar \
    gzip \
    ca-certificates && \
    dnf clean all

ARG K3S_VERSION=v1.31.5+k3s1
RUN curl -fsSL "https://github.com/k3s-io/k3s/releases/download/${K3S_VERSION}/k3s-arm64" \
        -o /usr/local/bin/k3s && chmod +x /usr/local/bin/k3s && \
    ln -sf /usr/local/bin/k3s /usr/local/bin/kubectl

COPY --from=build /out/microtunnel /out/tundial /usr/local/bin/
COPY --from=build /out/executor /app/executor

# Launch-time scripts (image/, copied into context by build.sh):
#   base-prep : common bring-up (kmsg shim, cgroup prep, microtunnel).
#   start     : the single entrypoint; dispatches on the baked $ROLE env var.
#   node-join : worker join step, invoked via /exec by up.sh.
COPY image/base-prep image/start image/node-join /usr/local/bin/
RUN chmod +x /usr/local/bin/base-prep /usr/local/bin/start /usr/local/bin/node-join

WORKDIR /app
EXPOSE 8080
CMD ["/usr/local/bin/start"]
