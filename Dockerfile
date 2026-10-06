FROM golang:tip-alpine3.24 AS builder

# Disable C compiler to create fully static binary
ENV CGO_ENABLED=0
# Ensure binary is compiled for Linux
ENV GOOS=linux

WORKDIR /go/src

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY main.go .

# - Use all CPU cores for parallel compilation
# - Strip debug symbols to reduce binary size
# - Remove file paths for security
RUN GOMAXPROCS=$(nproc) \
    go build \
    -ldflags="-w -s" \
    -trimpath \
    -buildvcs=false \
    -o container .

# Can't use scratch because we need a minimal base for the binary
# with a real filesystem, and busybox-static is needed since
# the container's isolation will not link dynamic libraries
# This ensures that the container has a minimal environment to run 
# shell tools like ls, cat, echo alongside the static binary.
FROM alpine:3.24
RUN apk add --no-cache busybox-static

COPY --from=builder /go/src/container /container
ENTRYPOINT ["/container"]