FROM --platform=$BUILDPLATFORM golang:1.26.3-alpine3.22@sha256:be93003ee861b3b91b6ebcb22678524947e0cd786c2df3f32af520006b1e54f5 AS builder
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

FROM alpine:3.23.4@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11
COPY --from=builder --chown=10001:0 /workspace/logexporter /logexporter
