FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN go build -o /bin/meeting-bot ./cmd/bot

FROM alpine:3.22

WORKDIR /app

COPY --from=builder /bin/meeting-bot /usr/local/bin/meeting-bot

CMD ["meeting-bot"]
