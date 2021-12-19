# builder image
FROM golang:latest as builder
RUN mkdir /build
ADD . /build/
WORKDIR /build/cmd
RUN CGO_ENABLED=0 GOOS=linux go build -a -o phantom .

# generate clean, final image for end users
FROM alpine:latest

COPY --from=builder /build/cmd/phantom .

ENV BIND=0.0.0.0
ENV BIND_PORT=19132
ENV TIMEOUT=60

# executable
ENTRYPOINT ./phantom -bind "$BIND" -bind_port "$BIND_PORT" -server "$SERVER" -timeout "$TIMEOUT" $EXTRA_ARGS
