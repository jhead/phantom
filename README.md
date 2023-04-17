# phantom - bedrock proxy for xbox & ps

This docker images simulates a LAN Server and proxies the packets to the real server.
Xbox & playstation user will see them as LAN Servers in the Serverlist.

To run multiple proxies just start another container.

## Run container with docker

```bash
docker run -d --env "SERVER=172.17.17.88:19132" --network host ghcr.io/andybroger/phantom:latest
```

## Run with docker compose

```yml
version: "3"

services:
  phantom:
    image: ghcr.io/andybroger/phantom:latest
    restart: always
    network_mode: host
    environment:
      SERVER: "172.17.17.88:19132"
```

## Build the container

```bash
docker build . -t phantom:latest
```

## Credits

Thanks to [Justin Head](https://github.com/jhead/phantom) for developing the phantom proxy!
