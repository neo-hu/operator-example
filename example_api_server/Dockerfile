# Build the manager binary
FROM golang:1.20 as builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy the go source
COPY example_api_server/ example_api_server/
ENV GOPROXY https://goproxy.cn
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o /api_server example_api_server/main.go

FROM alpine:3.15.4
WORKDIR /
COPY --from=builder /api_server .
EXPOSE 8111
ENTRYPOINT ["/api_server"]
