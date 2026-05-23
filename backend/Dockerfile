FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/bin/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/bin/worker ./cmd/worker

FROM alpine:3.19

RUN apk add --no-cache tzdata

WORKDIR /app

COPY --from=builder /app/bin/api /app/api
COPY --from=builder /app/bin/worker /app/worker
COPY configs /app/configs

RUN chmod +x /app/api /app/worker

EXPOSE 8080

CMD ["/app/api"]
