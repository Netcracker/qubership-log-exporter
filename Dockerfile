FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine3.22@sha256:727cfc3c40be55cd1bc9a4a059406b28a059857e3be752aa9d09531e12c20c56 AS builder
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

ARG GOPROXY=""
ENV GO111MODULE=on

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum

COPY *.go ./
COPY internal/ internal/

RUN go mod download -x

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -o logexporter .

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
COPY --from=builder --chown=10001:0 /workspace/logexporter /logexporter
