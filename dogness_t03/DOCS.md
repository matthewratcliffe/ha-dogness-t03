# Dogness Treater (T03)

Camera feed + treat dispensing for Dogness TUTK cameras, served through go2rtc.

## Configuration

```yaml
username: you@example.com   # Dogness app email
password: yourpassword      # Dogness app password
timezone: 11                # UTC offset in hours; 0 = auto-detect
stream: treater             # go2rtc stream name for the camera
mqtt_camera: true           # publish the snapshot camera over MQTT
log_level: info             # trace | debug | info | warn | error
```

Only `username` and `password` are required. The add-on logs in once, reads your feeder's
UID + device key from the Dogness device list, discovers its LAN IP, and registers a camera
stream automatically.

## Ready-made entities (MQTT)

If the **Mosquitto broker** add-on (the `mqtt` service) is available, the add-on publishes
Home Assistant **MQTT discovery** messages so a device **Dogness Treater** appears
automatically with:

- `camera.dog_treater` — a snapshot camera (~1–2 fps) pushed over MQTT.
- `button.treat` — press to dispense a treat.

No YAML or manual integration is needed; add them to a dashboard by picking them from the
entity list. Set `mqtt_camera: false` to publish only the button.

> The MQTT camera is a **snapshot** camera (~1 fps). For smooth, full-rate live video use the
> bundled **Dogness Live card** below.

## Live video widget (smooth, mobile-friendly)

The add-on ships a self-contained Lovelace card that plays the go2rtc stream **directly in your
browser/app** (MSE/WebRTC, no iframe, no HACS). It renders inline so it works in the Home
Assistant mobile app, where iframe/Webpage cards do not.

**It sets itself up.** On start the add-on:
1. copies `dogness-card.js` into HA's `www` folder, and
2. auto-registers it as a frontend resource via HA's API.

So you don't add a resource or write any YAML. Just add the card from the UI:

- Edit a dashboard → **+ Add Card** → scroll to **"Dogness Live"** in the card picker → add it.
  It works with **no configuration** (the host is auto-detected from how you're viewing HA).

That's it — a smooth live widget alongside the snapshot `camera.dog_treater` and `button.treat`.

> **Requirements / caveats**
> - Auto-registration needs dashboards in the default (storage) mode. If the card isn't in the
>   picker, check the add-on log; you can also add it by hand: **Settings → Dashboards → ⋮ →
>   Resources → + Add** `/local/dogness-card.js` (type **JavaScript Module**), then refresh.
> - Works when you open HA over **http** on the LAN. Over **https** (e.g. Nabu Casa) the browser
>   blocks the `ws://` connection to the add-on (mixed content) — on mobile use the snapshot
>   camera instead, or front go2rtc with an https reverse proxy.
> - To point at a non-default stream, edit the card and set `src: <stream name>`.

MQTT topics used: `homeassistant/button/dogness_<stream>/config`,
`homeassistant/camera/dogness_<stream>/config`, command `dogness/<stream>/treat/set`,
image `dogness/<stream>/image`, availability `dogness/<stream>/status`.

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
