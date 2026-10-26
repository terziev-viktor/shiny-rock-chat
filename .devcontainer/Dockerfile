FROM golang:1.25.14 AS go-sdk

FROM ghcr.io/cirruslabs/flutter:stable

USER root

COPY --from=go-sdk /usr/local/go /usr/local/go

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        build-essential \
        clang \
        cmake \
        git \
        vim \
        gstreamer1.0-plugins-base \
        gstreamer1.0-plugins-good \
        libgstreamer1.0-dev \
        libgstreamer-plugins-base1.0-dev \
        libgl1-mesa-dev \
        libgtk-3-dev \
        liblzma-dev \
        libpulse-dev \
        libsecret-1-dev \
        ninja-build \
        pkg-config \
    && rm -rf /var/lib/apt/lists/*

ENV GOPATH=/go
ENV PATH="/usr/local/go/bin:/go/bin:${PATH}"
