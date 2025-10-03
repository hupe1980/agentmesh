# syntax=docker/dockerfile:1
FROM mcr.microsoft.com/devcontainers/go:1-1.24-bookworm

ARG JUST_VERSION=1.35.0
ARG JEKYLL_VERSION=4.3.3
ARG BUNDLER_VERSION=2.5.14

ENV GOPATH=/go
ENV PATH=/go/bin:${PATH}

# Install dependencies and Jekyll
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        unzip \
        ruby-full \
        build-essential \
        zlib1g-dev \
    && echo 'gem: --no-document' >> /etc/gemrc \
    && gem install bundler -v ${BUNDLER_VERSION} \
    && gem install jekyll -v ${JEKYLL_VERSION} \
    && rm -rf /var/lib/apt/lists/*

# Install just
RUN ARCH="$(dpkg --print-architecture)" \
    && case "$ARCH" in \
        amd64) JUST_ARCH="x86_64" ;; \
        arm64) JUST_ARCH="aarch64" ;; \
        *) echo "unsupported architecture: $ARCH" && exit 1 ;; \
    esac \
    && curl -fsSL "https://github.com/casey/just/releases/download/${JUST_VERSION}/just-${JUST_VERSION}-${JUST_ARCH}-unknown-linux-musl.tar.gz" -o /tmp/just.tar.gz \
    && tar -xzf /tmp/just.tar.gz -C /tmp just \
    && install -m 0755 /tmp/just /usr/local/bin/just \
    && rm -rf /tmp/*
