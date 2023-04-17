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
ENV PORT=58682
ENV DEBUG=false
ENV SERVER=

# create a directory for the app
WORKDIR /app

# copy the binary from the builder
COPY --from=builder /app/phantom .

# run the app with the ENV variables
CMD ./phantom -bind_port=$PORT -debug=$DEBUG -server=$SERVER
