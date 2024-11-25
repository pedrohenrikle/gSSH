FROM alpine:latest

RUN apk update && apk add libc6-compat openssl

RUN mkdir -p /app/cert && mkdir -p /app/out

# generate keys and crt into cert dir
RUN openssl req -x509 -newkey rsa:4096 -keyout /app/cert/server.key -out /app/cert/server.crt -days 365 -nodes -subj "/CN=localhost"

COPY cmd /app

COPY pb /app
