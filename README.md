[![Downloads](https://img.shields.io/github/downloads/jhead/phantom/total)](https://github.com/jhead/phantom/releases) [![Gitter](https://badges.gitter.im/phantom-minecraft/community.svg)](https://gitter.im/phantom-minecraft/community?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge)

# phantom

Makes hosted Bedrock/MCPE servers show up as LAN servers, specifically for consoles.

You can now play on remote servers (not Realms!) on your Xbox and PS4 with friends.

It's like having a LAN server that's not actually there, spooky.

## Installing

phantom is a command line application with no GUI (yet). See the usage section below.

[Download](https://github.com/jhead/phantom/releases) phantom from the releases page.

**macOS / Linux**

Add execute persmissions if necessary:

```bash
$ chmod u+x ./phantom-<os>
```

Just replace `<os>` with macos, linux, etc. for the correct OS you're using.

## Usage

Open up a command prompt (Windows) or terminal (macOS & Linux) to the location
where you downloaded it, then the server should show up on your LAN list within
a few seconds. If not, you did something wrong. Or I did ;)

```
Usage: ./phantom-<os> [options] -server <server-ip>

Options:
  -bind string
    	IP address to listen on, port is randomized (default "0.0.0.0")
  -server string
    	Bedrock/MCPE server IP address and port (ex: 1.2.3.4:19132)
  -timeout int
    	Seconds to wait before cleaning up a disconnected client (default 60)
```

**Running multiple instances**

If you have multiple Bedrock servers, you can run phantom multiple times on
the same device to allow all of your servers to show up on the LAN list. All
you have to do is start one instance of phantom for each server and set the
`-server` flag appropriately. You don't need to use `-bind` or change the port.
But you probably do need to make sure you have a firewall rule that allows
all UDP traffic for the phantom executable.

**A note on `-bind`:**

The port is randomized and specifically omitted from the flag because the
port that phantom runs on is irrelevant to the user. phantom must bind to
port 19132 on all interfaces (or at least the broadcast address) to receive
ping packets from LAN devices. So phantom will always do that and there's no
way to configure otherwise, but you can also pick which IP you want the proxy
itself to listen on, just in case you need that. You shouldn't though.

As long as the device you run phantom from is on the same LAN, the default
settings should allow other LAN devices to see it when you open Minecraft.

**Example**

Connect to a server at IP `104.219.6.162` port `19132`:

```bash
$ ./phantom-<os> 104.219.6.162:19132
```

Same as above but bind to a specific local IP:

```bash
$ ./phantom-<os> -bind 10.0.0.5:19132 104.219.6.162:19132
```

## Building

Makefile builds for Windows, macOS, and Linux, including x86 and ARM.

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
- ARM builds are available for Raspberry Pi and similar SOCs.
- Minecraft for Windows 10, iOS/Android, Xbox One, and PS4 are currently supported.
- **Nintendo Switch is not supported.**

Note that you almost definitely need to create a firewall rule for this to work.
On macOS, you'll be prompted automatically. On Windows, you may need to go into
your Windows Firewall settings and open up all UDP ports for phantom.
