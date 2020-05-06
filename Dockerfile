FROM golang:1.14 as builder

COPY go.* /src/
WORKDIR /src
RUN go mod download

ADD . /src
RUN CGO_ENABLED=0 go build -o zabbix_elasticmq cmd/main.go

# -----

FROM alpine:latest

COPY --from=builder /src/zabbix_elasticmq /zabbix_elasticmq

CMD ["/zabbix_elasticmq"]
