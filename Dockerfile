FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git make

COPY . /src
WORKDIR /src

RUN make build

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /src/bin/ /app/

WORKDIR /app

EXPOSE 8000
EXPOSE 9000
VOLUME /data/conf

CMD ["./nonoka-im", "-conf", "/data/conf"]
