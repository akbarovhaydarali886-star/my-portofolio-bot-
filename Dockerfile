# Multi-stage build for lightweight Go container
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download || true

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o bot main.go

# Production runner image
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Tashkent

WORKDIR /app

COPY --from=builder /app/bot ./bot
RUN chmod 755 ./bot

ENV PORT=8080
EXPOSE 8080

CMD ["./bot"]
