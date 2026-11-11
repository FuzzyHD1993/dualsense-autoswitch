# dualsense-autoswitch

Switch **DualSense trigger/lightbar profiles automatically** based on the running
game — in a single static Go binary instead of a polling bash daemon.

The bash versions spent **~30 % of a CPU core** continuously: every 2 seconds they
spawned `tr`, `sort`, `head` and `dualsensectl` for every process on the system.
This daemon reads `/proc` directly, has no forks, and idles at **~0.3 %**.

## How it works

```
every 2s:
  read /proc/*/comm + basename(cmdline)   →  process names
  match against profiles.conf (globs)     →  profile name
  if changed → run ~/.local/bin/dualsense-profile-<name>
```

- Controller presence is detected through `/sys/class/hidraw/*/uevent`
  (Sony vendor `054C`), not by spawning `dualsensectl` every cycle.
- On controller reconnect the current profile is re-applied (BT drops effects).
- `~/.config/dualsense/override` pins a profile manually — it wins over the
  running-game match and survives reconnects. Use `auto`/empty to release it.
- `SIGHUP` reloads the config without a restart.

## Install

```bash
git clone https://github.com/FuzzyHD1993/dualsense-autoswitch
cd dualsense-autoswitch
./install.sh
```

The installer builds the binary, installs the user unit, seeds
`~/.config/dualsense/profiles.conf` (if absent) and installs the example
profile dispatcher.

## Configuration

`~/.config/dualsense/profiles.conf`:

```
AlanWake2.exe     | aw2
RDR2.exe          | rdr2
PathOfExile*      | poe2
eurotrucks2.exe   | ets2
```

- glob, **case-insensitive**
- checked against both `comm` and the executable name from `cmdline`
  (needed for Proton/Wine games)
- first match wins, otherwise `default`

## Profiles

A profile is just an executable script `~/.local/bin/dualsense-profile-<name>`.

`profiles/dualsense-variant` is a ready-made dispatcher you can symlink:

```bash
for p in aw2 rdr2 poe2 cd ets2 cp2077 default; do
    ln -sf dualsense-variant ~/.local/bin/dualsense-profile-$p
done
```

It uses every DualSense trigger mode, so profiles actually feel different:

| Mode | Effect |
|---|---|
| `feedback` | resistance from a position |
| `weapon` | free travel then a hard wall (gun trigger) |
| `bow` | tension and a snap (bowstring) |
| `machine` | pulsing resistance (automatic fire) |
| `slope_feedback` | progressively rising resistance (throttle) |
| `vibration` | buzz under your finger while held |
| `vibration-raw` | buzz shaped by trigger depth |

## Toggle

Some games (Crimson Desert) drive the triggers themselves through SteamInput
and reset external effects. Instead of fighting them, flip the switch:

```bash
dualsense-toggle        # off: release the controller to the game
dualsense-toggle        # on:  re-apply the per-game profile
```

Bind it to a key — the marker file is `~/.config/dualsense/disabled`.
While disabled the daemon applies `default` once and stays out of the way.

## Panel widget

[dualsense-tray](https://github.com/FuzzyHD1993/dualsense-tray) puts a gamepad
icon into the COSMIC status area; one click toggles the profiles.

## Override

```bash
echo poe2 > ~/.config/dualsense/override   # pin
: > ~/.config/dualsense/override           # back to auto
```

## Debug

```bash
dualsense-autoswitch -once     # print the profile that would be applied
dualsense-autoswitch -v        # log the choice every cycle
```

## Configuration via environment

| Variable | Default |
|---|---|
| `DS_CONF` | `~/.config/dualsense/profiles.conf` |
| `DS_OVERRIDE` | `~/.config/dualsense/override` |
| `DS_BIN` | `~/.local/bin` |
| `DS_INTERVAL_MS` | `2000` |

## Limitations

- Profiles are static per game: a PC cannot know which weapon you hold without
  reading game memory (which is a ban risk), so effects do not change dynamically.
- Body rumble is not exposed by `dualsensectl`; only trigger effects.

## License

MIT
