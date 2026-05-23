FROM alpine:3.19

RUN apk add --no-cache tzdata

WORKDIR /app

COPY bin/api /app/api
COPY bin/worker /app/worker
COPY configs /app/configs

RUN chmod +x /app/api /app/worker

EXPOSE 8080

CMD ["/app/api"]
