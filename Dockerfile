# Multi-stage build for lightweight Go container
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download || true

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bot main.go

# Production runner image (under 20MB)
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Tashkent

WORKDIR /app

COPY --from=builder /app/bot /app/bot
RUN chmod +x /app/bot

ENV PORT=8080

EXPOSE 8080

ENTRYPOINT ["/app/bot"]
