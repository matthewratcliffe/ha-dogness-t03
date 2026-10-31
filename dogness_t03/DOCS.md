# Dogness Treater (T03)

Camera feed + treat dispensing for Dogness TUTK cameras, served through go2rtc.

## Configuration

```yaml
username: you@example.com   # Dogness app email
password: yourpassword      # Dogness app password
timezone: 11                # UTC offset in hours; 0 = auto-detect
stream: treater             # go2rtc stream name for the camera
log_level: info             # trace | debug | info | warn | error
```

Only `username` and `password` are required. The add-on logs in once, reads your feeder's
UID + device key from the Dogness device list, discovers its LAN IP, and registers a camera
stream automatically.

## Ports

| Port | Use |
|---|---|
| 1984/tcp | go2rtc web UI / API / treat endpoint |
| 8554/tcp | RTSP |
| 8555/tcp + udp | WebRTC |

The add-on runs with **host networking** (required to broadcast-discover the feeder on your
LAN). Reach it from Home Assistant at your HA host's IP, or the Supervisor gateway
`172.30.32.1`.

## Treat endpoint

```
POST http://<HA_IP>:1984/api/dogness/feed?id=<stream>
```

- `id` is the stream name (default `treater`). If you have exactly one feeder you may omit it.
- Returns `{"status":"ok"}` on success.

## Camera + button

See the repository [README](https://github.com/matthewratcliffe/ha-dogness-t03) for the
`generic` camera, `rest_command`, and dashboard-card YAML.

## Multiple feeders

If your account has more than one device, each gets its own stream. The first uses the
`stream` name; others are named from the device name (e.g. `smart_cam_treater_t03-2`). Check
the add-on log for the exact names, then use `?id=<name>` for each treat button.

## Troubleshooting

- **No `feeder ready` line in the log / login fails** — check the email/password, and that the
  add-on has internet at startup (needed only to fetch the device list).
- **`device ... not found on LAN`** — the feeder must be powered on and on the same subnet as
  Home Assistant. Host networking must be enabled (it is by default here).
- **Camera won't load but the go2rtc Web UI plays it** — your camera `stream_source` URL/IP is
  wrong; use the HA host IP or `172.30.32.1`, and `rtsp_transport: tcp`.
- **Treat says ok but nothing drops** — make sure the feeder has treats and isn't jammed; the
  command is confirmed working end-to-end, the hardware simply has to cooperate.
- Set `log_level: debug` for verbose output.
