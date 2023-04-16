FROM golang:alpine3.17 as builder

# create a directory for the app
WORKDIR /app

# copy the source code
COPY . .

# install dependencies
RUN go mod download

# build the app
RUN go build -o phantom cmd/phantom.go

FROM alpine:3.17

# prepare ENV variables
#ENV PHANTOM_IPV6=false
ENV PHANTOM_BIND=0.0.0.0
ENV PHANTOM_PORT=58682
ENV PHANTOM_DEBUG=false
ENV PHANTOM_SERVER=FALSE
#ENV PHANTOM_REMOVE_PORTS=32323

# create a directory for the app
WORKDIR /app

# copy the binary from the builder
COPY --from=builder /app/phantom .

# run the app with the ENV variables
CMD ./phantom -bind=$PHANTOM_BIND -bind_port=$PHANTOM_PORT -debug=$PHANTOM_DEBUG -server=$PHANTOM_SERVER
