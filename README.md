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

Open **Dogness Treater (T03) → Install**. It installs in seconds — the add-on ships a
prebuilt, statically-linked go2rtc binary per architecture (no on-device compile).

### 3. Configure

On the **Configuration** tab:

| Option | Value |
|---|---|
| `username` | your Dogness app email |
| `password` | your Dogness app password |
| `timezone` | your UTC offset in hours (e.g. `11` for Sydney, `-5` for New York). `0` = auto-detect |
| `stream` | camera stream name, default `treater` |
| `mqtt_camera` | publish the snapshot camera over MQTT (default on) |

> For the **ready-made entities** below, also install the **Mosquitto broker** add-on (and the
> **MQTT** integration). Without MQTT the add-on still serves video + the treat API; you just
> wire them up with the YAML fallback.

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

## Ready-made entities (no YAML)

> ✅ **Requires the [Mosquitto broker](https://github.com/home-assistant/addons/tree/master/mosquitto)
> add-on** (one-click install from the Add-on Store, then add the **MQTT** integration if HA
> doesn't auto-prompt). With it, the add-on publishes its entities to Home Assistant
> automatically — no YAML, no manual integration.

Once the add-on is running with MQTT available, a device named **Dogness Treater** appears
under **Settings → Devices & Services → MQTT**, with two ready-made entities:

| Entity | What it is |
|---|---|
| `camera.dog_treater` | the camera feed |
| `button.treat` | press to dispense a treat |

**Add them to a dashboard by picking them from the list:** edit a dashboard → **+ Add Card**
→ choose **Picture glance / Picture entity** and pick `camera.dog_treater`, and **Button** and
pick `button.treat`. No YAML required.

> 📷 **About the camera.** The ready-made MQTT camera shows a live snapshot stream (~1 fps) —
> perfect for a glanceable tile and reliable everywhere (incl. the mobile app). For **smooth,
> full-rate live video**, add the bundled **Dogness Live card** ([below](#smooth-live-video-dogness-live-card));
> both can coexist.

### Don't have MQTT? (YAML fallback)

If you'd rather not run a broker, you can wire everything manually. The add-on uses host
networking, so reach it at **your Home Assistant host's IP** (`HA_IP`, the address you use to
open HA) or the Supervisor gateway `172.30.32.1`. In `configuration.yaml`:

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

Restart HA; you then have `camera.dog_treater` and the `rest_command.dogness_treat` action to
put on a dashboard.

### Smooth live video (Dogness Live card)

The MQTT camera is snapshot-based. For **smooth, full-rate live video** — including inside the
**Home Assistant mobile app**, where iframe/Webpage cards don't render — the add-on ships a
self-contained Lovelace card (`dogness-live`). It plays the go2rtc stream **directly from your
browser/app** (MSE/WebRTC), so it needs no HACS, no WebRTC integration, and no iframe.

**It sets itself up** — on start the add-on copies `dogness-card.js` into HA's `www` folder
*and* auto-registers it as a frontend resource (via HA's API). No YAML, no resource step.

Just add it from the UI: edit a dashboard → **+ Add Card** → pick **"Dogness Live"** from the
card gallery. It needs **no configuration** (the host is auto-detected from how you're viewing
HA). It sits alongside the snapshot `camera.dog_treater` and `button.treat`.

> - Auto-registration needs default (storage-mode) dashboards. If the card isn't in the picker,
>   add it by hand once: **Settings → Dashboards → ⋮ → Resources → + Add** `/local/dogness-card.js`
>   (type **JavaScript Module**), then refresh.
> - Works over **http** on the LAN. Over **https** (Nabu Casa) the browser blocks the `ws://`
>   to the add-on (mixed content) — use the snapshot camera on mobile there.

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
troubleshooting. The go2rtc source added for this device lives in
[`dogness_t03/overlay/`](dogness_t03/overlay/); the shipped binaries in `dogness_t03/bin/` are
built from it (upstream go2rtc @ `7c05066` + overlay) and are reproducible with
[`scripts/build.sh`](scripts/build.sh).

📄 **[RESEARCH.md](RESEARCH.md)** — a detailed technical report on the reverse-engineering:
the TUTK/Kalay protocol, the obfuscation scheme, the authentication and control semantics,
the cloud provisioning API, LAN discovery, and a security analysis.

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
