# phantom

Makes hosted Bedrock/MCPE servers show up as LAN servers, specifically for Xbox.

You can now play on remote servers (not Realms!) on your Xbox with friends.

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

Open up a command prompt (Windows) or terminal (macOS & Linux) to the location where you downloaded it.

```bash
Usage: ./phantom-<os> <server-ip>

Options:
  -bind string
    	Bind address and port (default "0.0.0.0:19132")
```

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
- Only Minecraft for Windows 10, iOS/Android, and Xbox are currently supported.
- **PS4 and Nintendo Switch are not supported.**

Note that you almost definitely need to create a firewall rule for this to work.
On macOS, you'll be prompted automatically. On Windows, you may need to go into
your Windows Firewall settings and open up port 19132 (UDP).
