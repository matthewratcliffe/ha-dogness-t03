# Dogness T03 — Home Assistant add-on

Live **camera feed** and **treat dispensing** for the **Dogness Smart Cam Treater T03**
(model **T1020**) and other Dogness TUTK/Kalay cameras — as a native Home Assistant OS
add-on you can drop onto a dashboard.

- 📹 **Camera** — H.264 video exposed as RTSP / WebRTC / HLS / snapshots via [go2rtc](https://github.com/AlexxIT/go2rtc).
- 🦴 **Treat** — a simple `POST` endpoint drops a treat; wire it to a dashboard button.
- 🏠 **Local** — talks the feeder's P2P protocol **directly over your LAN**. Your Dogness
  login is used **once at startup** only to fetch the device's id + key; video and treats
  never leave your network after that. No phone app, no cloud streaming, no separate bridge.
- 🧩 **Pure Go** — a small go2rtc build with a custom `dogness://` source compiled in.
  No native vendor libraries.

> ℹ️ You only enter your **Dogness account email + password** in the add-on settings.
> It logs in, finds your feeder, and **auto-discovers its LAN IP** itself.

---

## Compatibility

| | |
|---|---|
| **Devices** | Dogness **Smart Cam Treater T03** / model **T1020**. Other Dogness TUTK cameras may work. |
| **Home Assistant** | Home Assistant **OS** or **Supervised** (anything with the Add-on Store). |
| **Architectures** | `aarch64` (Pi 4/5 64-bit), `amd64`, `armv7`. |
| **Network** | The add-on and the feeder must be on the **same LAN** (uses UDP broadcast to find the device). |

---

## Install

### 1. Add this repository

In Home Assistant: **Settings → Add-ons → Add-on Store → ⋮ (top-right) → Repositories**,
paste:

```
https://github.com/matthewratcliffe/ha-dogness-t03
```

Click **Add**, close, then refresh. A **Dogness T03 Add-ons** section appears.

### 2. Install the add-on

Open **Dogness Treater (T03) → Install**. The first build compiles go2rtc from source and
takes a few minutes (longer on a Pi).

### 3. Configure

On the **Configuration** tab:

| Option | Value |
|---|---|
| `username` | your Dogness app email |
| `password` | your Dogness app password |
| `timezone` | your UTC offset in hours (e.g. `11` for Sydney, `-5` for New York). `0` = auto-detect |
| `stream` | camera stream name, default `treater` |

Save, then **Start** the add-on. On the **Log** tab you should see:

```
feeder ready: stream="treater" name="Smart Cam Treater T03" model=T1020 uid=...
```

> 💡 Give the feeder a **static DHCP lease** in your router. The add-on re-discovers its IP
> automatically, but a stable lease avoids any hiccups.

### 4. (Optional) check it works

Open the add-on's **Web UI** (go2rtc, port `1984`). Your `treater` stream should be listed
and playable in the browser.

---

## Add the camera to a dashboard

The add-on uses host networking, so Home Assistant reaches it at **your Home Assistant box's
own IP address** (the same IP you use to open HA). Replace `HA_IP` below with it
(e.g. `192.168.1.50`).

Add to `configuration.yaml`:

```yaml
camera:
  - platform: generic
    name: Dog Treater
    stream_source: rtsp://HA_IP:8554/treater
    still_image_url: http://HA_IP:1984/api/frame.jpeg?src=treater
    rtsp_transport: tcp

rest_command:
  dogness_treat:
    url: "http://HA_IP:1984/api/dogness/feed?id=treater"
    method: POST
    timeout: 25
```

> Alternative to `HA_IP`: from Home Assistant Core you can also use the Supervisor host
> gateway `172.30.32.1` (e.g. `rtsp://172.30.32.1:8554/treater`) if you'd rather not hardcode
> your LAN IP.

Restart Home Assistant (or reload **Generic Camera** / **REST commands**). You now have
`camera.dog_treater` and the action `rest_command.dogness_treat`.

### Dashboard card (camera + treat button)

Add a **Manual** card:

```yaml
type: vertical-stack
cards:
  - type: picture-entity
    entity: camera.dog_treater
    camera_view: live
    show_state: false
    show_name: false
  - type: button
    name: Drop a treat 🦴
    icon: mdi:bone
    tap_action:
      action: perform-action
      perform_action: rest_command.dogness_treat
```

Tap the button → a treat drops. 🎉

---

## How it works

1. **Cloud login (once):** at startup the add-on logs into the Dogness app API and reads your
   feeder's P2P **UID** and **device key** from the device list. Nothing is streamed through
   the cloud.
2. **LAN discovery:** it finds the feeder's IP by broadcasting a TUTK/Kalay LAN search
   (UDP `:32761`) and matching the reply to your UID. This is why it needs host networking.
3. **Direct connect:** it opens a TUTK session straight to the feeder, starts the H.264
   stream, and serves it through go2rtc.
4. **Treat:** `POST /api/dogness/feed?id=<stream>` sends the feeder's treat command
   (IOCtrl `8318`) over an active session — reusing the live view if open, otherwise opening a
   brief session just to dispense.

See [`dogness_t03/DOCS.md`](dogness_t03/DOCS.md) for ports, multiple feeders, and
troubleshooting, and [`dogness_t03/overlay/`](dogness_t03/overlay/) for the source added to
go2rtc.

---

## Privacy & credentials

Your Dogness email/password are stored only in this add-on's options on **your** Home
Assistant, and are sent only to the official Dogness API to retrieve your device list. The
feeder's UID and key likewise stay on your instance. If you reverse-engineered/recovered these
and your account password has appeared anywhere insecure, consider rotating it.

## Credits & licence

Built on [**go2rtc**](https://github.com/AlexxIT/go2rtc) by @AlexxIT (MIT), whose `pkg/tutk`
TUTK/Kalay implementation made this possible. This add-on and the `dogness://` source are
released under the **MIT** licence — see [LICENSE](LICENSE). Not affiliated with or endorsed
by Dogness.
