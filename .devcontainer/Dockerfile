# syntax=docker/dockerfile:1
FROM golang:1.24-bookworm

ARG USERNAME=vscode
ARG USER_UID=1000
ARG USER_GID=${USER_UID}
ARG GOLANGCI_LINT_VERSION=v1.60.3
ARG JUST_VERSION=1.35.0

ENV GOPATH=/go
ENV PATH=/go/bin:${PATH}

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        gnupg \
        sudo \
        procps \
        unzip \
        ruby-full \
        build-essential \
        zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

RUN echo 'gem: --no-document' >> /etc/gemrc \
    && gem install bundler jekyll

# Install just from upstream release since Debian repositories may lag behind
RUN ARCH="$(dpkg --print-architecture)" \
    && case "$ARCH" in \
        amd64) JUST_ARCH="x86_64" ;; \
        arm64) JUST_ARCH="aarch64" ;; \
        *) echo "unsupported architecture: $ARCH" && exit 1 ;; \
    esac \
    && curl -fsSL "https://github.com/casey/just/releases/download/${JUST_VERSION}/just-${JUST_VERSION}-${JUST_ARCH}-unknown-linux-musl.tar.gz" -o /tmp/just.tar.gz \
    && tar -xzf /tmp/just.tar.gz -C /tmp just \
    && install -m 0755 /tmp/just /usr/local/bin/just \
    && rm -rf /tmp/just /tmp/just.tar.gz

# Create a non-root user that matches the VS Code defaults
RUN groupadd --gid ${USER_GID} ${USERNAME} \
    && useradd --uid ${USER_UID} --gid ${USER_GID} -m ${USERNAME} -s /bin/bash \
    && mkdir -p ${GOPATH}/bin \
    && chown -R ${USERNAME}:${USERNAME} ${GOPATH} \
    && echo "${USERNAME} ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/${USERNAME} \
    && chmod 0440 /etc/sudoers.d/${USERNAME}

# Install golangci-lint in GOPATH and expose via /usr/local/bin
RUN go install github.com/golangci/golangci-lint/cmd/golangci-lint@${GOLANGCI_LINT_VERSION} \
    && ln -s ${GOPATH}/bin/golangci-lint /usr/local/bin/golangci-lint \
    && chown -R ${USERNAME}:${USERNAME} ${GOPATH}

# Default user
USER ${USERNAME}
