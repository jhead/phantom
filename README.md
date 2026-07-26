[![Downloads](https://img.shields.io/github/downloads/jhead/phantom/total)](https://github.com/jhead/phantom/releases) [![Gitter](https://badges.gitter.im/phantom-minecraft/community.svg)](https://gitter.im/phantom-minecraft/community?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge)

# phantom

Makes hosted Bedrock/MCPE servers show up as LAN servers, specifically for consoles.

You can now play on remote servers (not Realms!) on your Xbox and PS4 with friends.

It's like having a LAN server that's not actually there, spooky.

## Installing

phantom is a command line application with no GUI (yet). See the usage section below.

[Download](https://github.com/jhead/phantom/releases) phantom from the releases page.

**macOS / Linux**

Add execute permissions if necessary:

```bash
$ chmod u+x ./phantom-<os>
```

Just replace `<os>` with macos, linux, etc. for the correct OS you're using.

**Raspberry Pi / ARM**

Pick the binary from your *OS* architecture (`uname -m`), not the marketing
CPU name. A Pi 3/4/5 is "ARMv8" hardware, but a 32-bit Raspberry Pi OS still
needs the 32-bit build:

| `uname -m` | Download |
|---|---|
| `aarch64` or `arm64` | `phantom-linux-arm8` (alias: `phantom-linux-arm64`) |
| `armv7l` | `phantom-linux-arm7` |
| `armv6l` (Pi Zero / Pi 1) | `phantom-linux-arm6` |

`phantom-linux-arm8` is **64-bit only**. Using it on 32-bit Pi OS typically
fails with `Illegal instruction` or `Exec format error` — use `arm7` (or
`arm6`) instead.

**Termux on Android**

Termux can run the regular **Linux ARM** builds — you do **not** need a
`GOOS=android` binary (that requires the Android NDK). Pick by `uname -m`:

| `uname -m` | Download |
|---|---|
| `aarch64` or `arm64` | `phantom-linux-arm8` |
| `armv7l` | `phantom-linux-arm7` |
| older / unsure | `phantom-linux-arm5` (widest compatibility) |

Do **not** use plain `phantom-linux` (that is amd64). Copy the binary into
Termux home (`$HOME`) — shared storage under `/storage/...` is often mounted
`noexec`, which causes `Permission denied` even after `chmod`. Then:

```bash
cd $HOME
chmod u+x ./phantom-linux-arm8   # or arm7 / arm5
./phantom-linux-arm8 -server example.com:19132
```

Run the binary as its own command; do not append `-server` to `cd`.

**iSH on iOS**

iSH provides a 32-bit x86 Linux userspace (`uname -m` is typically `i686` or
`x86_64` under emulation of 32-bit userspace — use the x86 build). Download or
build `phantom-linux-x86`, not the ARM or amd64 Linux binaries:

```bash
chmod u+x ./phantom-linux-x86
./phantom-linux-x86 -server example.com:19132
```

Build it yourself with:

```bash
make bin/phantom-linux-x86
# or: CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -o bin/phantom-linux-x86 ./cmd
```

## Usage

Open up a command prompt (Windows) or terminal (macOS & Linux) to the location
where you downloaded phantom, then type in the phantom command and hit enter.
The server should show up on your LAN server list within a few seconds. If not,
you did something wrong. Or I did ;)

```
Usage: ./phantom-<os> [options] -server <server-ip>

Options:
  -6	Optional: Same as -ipv6 (legacy; broken in PowerShell — use -ipv6)
  -bind string
    	Optional: IP address to listen on. Defaults to all interfaces. (default "0.0.0.0")
  -bind_port int
    	Optional: Port to listen on. Defaults to 0, which selects a random port.
    	Note that phantom always binds to port 19132 as well, so both ports need to be open.
  -debug
    	Optional: Enables debug logging
  -ipv6
    	Optional: Enables IPv6 support on port 19133 (experimental)
  -remove_ports
    	Optional: Forces ports to be excluded from pong packets (experimental)
  -server string
    	Required: Bedrock/MCPE server IP address and port (ex: 1.2.3.4:19132)
  -timeout int
    	Optional: Seconds to wait before cleaning up a disconnected client (default 60)
```

**Example**

Connect to a server at IP `lax.mcbr.cubed.host` port `19132`:

```bash
./phantom-<os> -server lax.mcbr.cubed.host:19132
```

![fVoNSdU](https://user-images.githubusercontent.com/360153/85956959-18003880-b93e-11ea-811e-424b33b98528.png)

Same as above but bind to a specific local IP:

```bash
./phantom-<os> -bind 10.0.0.5 -server lax.mcbr.cubed.host:19132
```

Same as above but bind the proxy server to port 19133:
   
```bash
./phantom-<os> -bind_port 19133 -server lax.mcbr.cubed.host:19132
```

Same as above but bind the proxy server to local IP 10.0.0.5 and port 19133:
   
```bash
./phantom-<os> -bind 10.0.0.5 -bind_port 19133 -server lax.mcbr.cubed.host:19132
```

**Running multiple servers**

If you have multiple Bedrock servers, pass each one to a **single** phantom
process with repeated or comma-separated `-server` flags:

```bash
./phantom-<os> -server 192.168.1.13:19134 -server 192.168.1.13:19136
# or
./phantom-<os> -server 192.168.1.13:19134,192.168.1.13:19136
```

Phantom binds LAN discovery (`:19132`) once and answers for every upstream
server, so they all appear in the Friends/LAN list at the same time. Running
multiple phantom *processes* cannot share `:19132` correctly — only one will
see traffic — so use multiple `-server` flags instead. You probably also need
a firewall rule that allows all UDP traffic for the phantom executable.

**A note on `-bind`:**

The port is randomized by default and specifically omitted from the flag because
the port that phantom runs on is irrelevant to the user. phantom must bind to
port 19132 on all interfaces (or at least the broadcast address) to receive
ping packets from LAN devices. So phantom will always do that and there's no
way to configure otherwise, but you can also pick which IP you want the proxy
itself to listen on, just in case you need that. You shouldn't though.

As long as the device you run phantom from is on the same LAN, the default
settings should allow other LAN devices to see it when you open Minecraft.

**A note on `-bind_port`:**

The port used by the proxy server can be defined with the `-bind_port` flag.
It can be useful if you are behind a firewall or using Docker and want to open only
necessary ports to phantom. Note that you'll always need to open port 19132 in addition
to the bind port for phantom to work.

This flag can be used with or without the `-bind` flag. 
Default value is 0, which means a random port will be used.

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
- A `phantom-linux-x86` (386) build is available for iSH on iOS.
- ARM builds are available for Raspberry Pi, Termux on Android, and similar SOCs (see Installing for which binary to use).
- Minecraft for Windows 10, iOS/Android, Xbox One, and PS4 are currently supported.
- **Nintendo Switch is not supported.**

Note that you almost definitely need to create a firewall rule for this to work.
On macOS, you'll be prompted automatically. On Windows, you may need to go into
your Windows Firewall settings and open up all UDP ports for phantom.

## Troubleshooting

**`Illegal instruction` (or `Exec format error`) on a Raspberry Pi**

You almost certainly downloaded the wrong ARM build. `phantom-linux-arm8` is
64-bit (`aarch64`). On 32-bit Raspberry Pi OS (`uname -m` shows `armv7l` or
`armv6l`), use `phantom-linux-arm7` or `phantom-linux-arm6` instead — even if
the board itself is ARMv8.

**`Permission denied` or `cannot execute binary file` in Termux**

Move the binary into `$HOME` (not `/storage/...`), `chmod u+x`, and use the
ARM Linux build that matches `uname -m` — not `phantom-linux` (amd64) and not
an Android/`GOOS=android` build. See Installing → Termux on Android.

**`cd: too many arguments`**

`cd` only changes directory. Run phantom separately, e.g.
`cd ~/phantom && ./phantom-linux-arm8 -server 1.2.3.4:19132`.

**`syntax error` / `unexpected ")"` when starting phantom in iSH**

You downloaded the wrong architecture. iSH needs `phantom-linux-x86` (linux/386).
ARM and amd64 Linux binaries look like garbage to the shell and produce syntax
errors. See Installing → iSH on iOS.

**`listen udp4 :19132: invalid argument` on iSH**

Older builds required SO_REUSEPORT, which iSH rejects. Current builds fall back
to a normal UDP listen when reuseport is unsupported.

**My server isn't showing up on the list but it's online and phantom is showing connections!**

Make sure "Visible to LAN players" is turn **ON** in your server's settings: *(below shows setting OFF)*

<img src="https://user-images.githubusercontent.com/42201487/81394390-25f5c200-9122-11ea-83ba-a24eea96c83b.png" width=350 />

More info:
- https://github.com/jhead/phantom/issues/80#issuecomment-625737070
- https://github.com/jhead/phantom/issues/29#issuecomment-612808296
