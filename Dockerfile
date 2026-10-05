FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine3.24@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS builder
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

FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
COPY --from=builder --chown=10001:0 /workspace/logexporter /logexporter
