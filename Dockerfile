FROM --platform=$BUILDPLATFORM golang:1.24.2-alpine3.21 AS builder
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

ARG GOPROXY=""
ENV GOSUMDB=off \
    GO111MODULE=on

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum

COPY *.go ./
COPY internal/ internal/

RUN go mod download -x

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -o logexporter .

FROM alpine:3.21.3
COPY --from=builder --chown=10001:0 /workspace/logexporter /logexporter
