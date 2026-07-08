FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal

RUN go build -o /bin/meeting-bot ./cmd/bot

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/meeting-bot /usr/local/bin/meeting-bot

CMD ["meeting-bot"]
