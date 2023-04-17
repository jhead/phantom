# phantom docker image

Phantom docker image based on alpine linux.

## Run container with docker

```bash
docker run --env "SERVER=172.17.17.88:19132" --network host phantom
```

## Run with docker compose

```yml
version: "3"

services:
  phantom:
    image: andybroger/phantom:latest
    restart: always
    network_mode: host
    environment:
      SERVER: "172.17.17.88:19132"
```

## Build the container

```bash
docker build . -t phantom:latest
```
