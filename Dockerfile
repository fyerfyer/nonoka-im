FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-X main.Version=docker" -o /src/bin/nonoka-im ./cmd/nonoka-im
RUN CGO_ENABLED=0 go build -ldflags "-X main.Version=docker" -o /src/bin/msgworker ./cmd/msgworker

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /src/bin/nonoka-im /src/bin/msgworker /app/

WORKDIR /app

EXPOSE 8000
EXPOSE 9002
VOLUME /data/conf

CMD ["/app/nonoka-im", "-conf", "/data/conf"]
