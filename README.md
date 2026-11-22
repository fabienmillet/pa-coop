# pa-coop: self-hosted co-op for Prison Architect

A replacement for Prison Architect's online co-op backend, plus a small client mod.

- **Self-hosted**: your own server instead of the Photon cloud. Run it as a dedicated server, or let the game start it when you host.
- **More players**: up to 16 per game by default (tested with 13); the game itself limits a game to 4.
- **Sync that holds**: when the game detects a desync (`Object List Sync Error`), the mod reloads the host's prison in a couple of seconds instead of kicking the player.
- **Easy to install**: copy three files next to the game. Steam and Epic builds play together.

Works with the current version of the game (The_Jailhouse_1.02, r11056), Windows (Steam, Epic) and Linux (Steam).

## For players

### Quick install

- **Windows** (Steam or Epic): download [`install-pa-coop.bat`](https://github.com/fabienmillet/prison-architect-server/releases/latest/download/install-pa-coop.bat)
  and double-click it. It finds the game, installs the mod and asks whether you host or join (and the address to join).
  Run it again to switch mode or uninstall.
- **Linux** (Steam, native version):
  `curl -fsSL https://raw.githubusercontent.com/fabienmillet/prison-architect-server/master/install/install.sh | bash`

### Manual install

Download `pa-coop-windows.zip` (or `pa-coop-linux.tar.gz`) from the releases and copy its files into the game folder,
next to `Prison Architect64.exe`:

| File | Role |
|---|---|
| `dbghelp.dll` (Windows) / `pacoop.so` (Linux) | the mod: redirects online play to your server, auto-resync |
| `pacoop-server.exe` / `pacoop-server` | the server, started by the mod when you host |
| `pacoop.txt` | settings |

Linux only: add `LD_PRELOAD="./pacoop.so" %command%` to the game's Steam launch options.

Then edit `pacoop.txt`:

```ini
# Host a game: the mod starts the server (and asks your router to open the ports via UPnP)
server=local

# Join a friend or a dedicated server (IP or name, optional port of its name server, 4533 by default)
server=203.0.113.5
server=pa.example.org:14533

# Or choose in the game's "Region" menu (Go Online / Join Online Game)
servers=Friends=pa.example.org, Local=local
```

Play as usual: **Go Online** to host, **Join Online Game** to join. Without a `server`/`servers` line the game uses
the official servers. To uninstall, delete the three files.

Playing over the Internet, the host must be reachable on TCP port 4533 (one port): UPnP (automatic), a port
forward on the router, or a VPN between friends (Tailscale, ZeroTier, NetBird...).

## Dedicated server

One TCP port (4533 by default). Players use `server=<address>` (or `server=<address>:<port>` with another port).

```sh
./pacoop-server                          # port 4533
./pacoop-server -port 14533 -max-players 24  # see ./pacoop-server -h
```

**Docker** (amd64 and arm64):

```sh
docker run -d --name pa-coop -p 4533:4533 -e PACOOP_MAX_PLAYERS=16 ghcr.io/fabienmillet/prison-architect-server
```

Environment variables: `PACOOP_PORT`, `PACOOP_MAX_PLAYERS`, `PACOOP_PUBLIC`, `PACOOP_UPNP`, `PACOOP_VERBOSE`.

**Pterodactyl / Pelican**: import the egg from [`deploy/pterodactyl`](deploy/pterodactyl/egg-pa-coop.json) or
[`deploy/pelican`](deploy/pelican/egg-pa-coop.json) (Admin > Eggs > Import), create a server with one port
allocation, start it. The install script downloads the latest release; set `VERSION` to pin one, reinstall to update.

## How it works

Prison Architect's co-op runs on Photon Realtime: a name server, a master server (lobby) and a game server that
relays messages, the host's game being the authority. `pacoop-server` (Go) speaks the same protocol (Protocol16 over
TCP, Diffie-Hellman + AES for authentication) and plays the three roles on a single port (the token handed to the
game tells which role its next connection is for).

The mod (a `dbghelp.dll` proxy on Windows, an `LD_PRELOAD` library on Linux):

- resolves the Photon names (`ns.exitgames.com`...) to your server and shifts the ports if needed;
- with `server=local`, starts `pacoop-server` and stops it with the game;
- with `servers=`, starts a small local name server that lists your servers in the "Region" menu;
- replaces the game's sync error handler: instead of disconnecting, it asks the host to resend the prison (the same
  message as when joining), and asks again if the transfer stalls. It checks the game's bytes first and turns itself
  off on another game version.

## Building

```sh
./package.sh   # needs Go, gcc and mingw-w64 (x86_64-w64-mingw32-gcc); output in dist/
```

- `server/`: `pacoop-server`, and `pacoop-bot` (fake players to load-test a game: `pacoop-bot -server host:4533 -room NAME -n 12`)
- `install/`: the player install scripts; `deploy/`: Pterodactyl and Pelican eggs; `Dockerfile`
- `client/`: the Windows and Linux mods
- `tools/p16dump.py`: decodes Photon traffic from a packet capture (needs tshark)

Testing the auto-resync: start the server with `-chaos-file /tmp/chaos`, then `echo 20 > /tmp/chaos` drops the host's
messages for 20 seconds, which makes the clients desync. Add `test=1` to `pacoop.txt` to also get a manual trigger
(create a file named `pacoop-resync-test` in the game folder).

## Notes

This is an independent interoperability project: no code or assets from Prison Architect or the Photon SDK are
included. Each player needs their own copy of the game. Prison Architect is a trademark of Paradox Interactive;
Photon is a trademark of Exit Games.
