FROM golang:1.9 AS builder

WORKDIR /go/src/github.com/boivie/sensor2prometheus
COPY . /go/src/github.com/boivie/sensor2prometheus

RUN go get -d -v
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo .

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /go/src/github.com/boivie/sensor2prometheus/sensor2prometheus .

ENTRYPOINT ["/root/sensor2prometheus"]
