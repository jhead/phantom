# bedrock-proxy

Makes hosted Bedrock/MCPE servers show up as LAN servers, specifically for Xbox and mobile.

You can now play on remote servers (not Realms!) on your Xbox with friends.

## Installing

*Download available soon.*

For now, you can clone and build it locally.

## Usage

```
Usage: ./proxy <server-ip>

Options:
  -bind string
    	Bind address and port (default "0.0.0.0:19132")
```

## Building

Makefile builds for macOS, Linux, and Windows.

```bash
make
```

## How does this work?

On Minecraft platforms that support LAN servers, the game will broadcast a
server ping packet to every device on the same network and display any valid
replies as connectable servers. This tool runs on your computer - desktop,
laptop, Raspberry Pi, etc. - and pretends to be a LAN server, acting as a proxy,
passing all traffic from your game through your computer and to the server
(and back), so that Minecraft thinks you're connected to a LAN server, but
you're really playing on a remote server. As soon as you start it up, you should
see the fake server listed under LAN and, upon selecting it, connect to the real
Bedrock/MCPE server hosted elsewhere.

For an optimal experience, run this on a device that is connected via ethernet
and not over WiFi, since a wireless connection could introduce some lag. Your
game device can be connected to WiFi. Your remote server can be running on a
computer, a VM, or even with a Minecraft hosting service.

## Supported platforms

- This tool should work on Windows, macOS, and Linux.
- Only Minecraft for Windows 10, iOS/Android, and Xbox are currently supported.
- Nintendo Switch does not currently have LAN server support.

Note that you almost definitely need to create a firewall rule for this to work
On macOS, you'll be prompted automatically. On Windows, you may need to go into
your Windows Firewall settings and open up port 19132 (UDP).

