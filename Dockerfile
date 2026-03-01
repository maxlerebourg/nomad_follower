FROM golang:1.25-alpine AS builder
ADD . /build
RUN cd /build && go install -mod=mod

FROM alpine:3.23
COPY --from=builder /go/bin/nomad_follower .
CMD ["./nomad_follower"]
