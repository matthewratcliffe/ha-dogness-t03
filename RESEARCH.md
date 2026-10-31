# Reverse-Engineering the Dogness Smart Cam Treater T03: A Pure-Go, Cloud-Free Home Assistant Integration for a TUTK/Kalay IoT Pet Feeder

**A technical report**

---

## Abstract

Consumer IoT pet-feeder cameras are typically usable only through a vendor mobile
application that relays audio/video and control through a proprietary peer-to-peer (P2P)
cloud. This couples basic functionality — viewing the camera, dispensing a treat — to an
opaque third-party service, precludes local automation, and offers no documented API. This
report documents the complete reverse-engineering of the **Dogness Smart Cam Treater T03**
(hardware model **T1020**), a camera-equipped treat dispenser built on the **TUTK/Kalay**
P2P platform, and the design of a clean-room, **pure-Go**, LAN-only integration for **Home
Assistant**. We describe the static and dynamic analysis methodology (APK decompilation,
native `.so` disassembly, an instrumented native bridge, and packet capture), the recovered
wire protocol (the IOTC session layer, the "Charlie" transcode obfuscation, the connection
handshake, the AV authentication path, the media framing, and the device-control IOCtrls
for streaming and treat dispensing), the cloud provisioning API, and a LAN auto-discovery
mechanism. We then present an implementation that extends the open-source `go2rtc` media
server with a `dogness://` source, requiring no native vendor libraries and no cloud media
relay, and validate it end-to-end against physical hardware. We close with a security
analysis of the device's transport and a discussion of limitations and reproducibility.

**Keywords:** IoT, reverse engineering, TUTK, Kalay, P2P, ThroughTek, H.264, Home
Assistant, go2rtc, protocol analysis, pet feeder.

---

## 1. Introduction

### 1.1 Motivation

The Dogness T03 ("Smart Cam Treater") is a Wi-Fi treat dispenser with an integrated
720p camera and night vision. Out of the box it is controllable only through the Dogness
mobile app, which connects to the device over the ThroughTek **TUTK/Kalay** P2P network.
Two capabilities are desirable for home automation:

1. **View the camera feed** inside a local dashboard, and
2. **Dispense a treat** on demand or via automation.

Neither is exposed through any documented interface. The objective of this work was to
recover enough of the device's protocol to reproduce both capabilities **without the vendor
app or cloud media relay**, in a form that installs cleanly into Home Assistant OS.

### 1.2 Contributions

- A documented account of the T03's TUTK/Kalay wire protocol as observed on this device,
  including the specific protocol-version markers that distinguish it from other TUTK
  cameras already supported by open-source tooling.
- Identification of the **AV-authentication** parameters (fixed account name, security
  mode) and the discovery, via native-code analysis, that an apparent connection timeout
  was in fact an **authentication rejection**.
- The **device-control IOCtrl** semantics for starting the video stream and for dispensing
  a treat, including the non-obvious requirement that a treat command is only honoured
  while a media session is actively streaming.
- The **Dogness cloud provisioning API** (login hashing scheme and device-list endpoint)
  that returns the device's P2P UID and AV key.
- A **LAN auto-discovery** method using the TUTK broadcast search, removing any need to
  hard-code the device's IP address.
- A clean-room, **pure-Go** implementation (no vendor `.so`) packaged as a Home Assistant
  add-on, and its end-to-end validation against hardware.

### 1.3 Scope and ethics

All work was performed on hardware and accounts owned by the author, for the purpose of
interoperability and local control of a device the author owns. No vendor services were
attacked, overloaded, or accessed on behalf of other users; the cloud API was used only to
read the author's own device list during provisioning. Device- and account-specific secrets
(UID, AV key, account credentials, and the vendor's TUTK SDK license key) are **redacted**
throughout and appear nowhere in the published repository.

---

## 2. Background and Related Work

### 2.1 The TUTK/Kalay platform

ThroughTek's **Kalay** (SDK family historically branded **TUTK**) is a widely deployed P2P
connectivity platform for IP cameras and other devices. Relevant sub-APIs:

- **IOTC** — session establishment and P2P transport over UDP, including NAT traversal via
  ThroughTek "master" servers and a LAN fast path.
- **AV** — an audio/video layer layered on an IOTC session, providing authenticated
  channels, frame framing, and bidirectional **IOCtrl** control messages.
- **RDT** — a reliable-data transport used by some applications for file-like transfers.

The platform has been the subject of prior security research (notably CVE-2021-28372, an
IOTC session-hijack class issue) and of open-source interoperability efforts. Most relevant
here, the **`go2rtc`** media server contains a `pkg/tutk` implementation originally developed
to support Wyze cameras, which speak a TUTK dialect. That package provided the foundation on
which this work builds; the central reverse-engineering problem was to determine *how the
T03's dialect differs* and to supply the device-specific control semantics.

### 2.2 go2rtc

`go2rtc` is an MIT-licensed, single-binary streaming server that ingests many camera
protocols and republishes them as RTSP, WebRTC, HLS, MJPEG, and still snapshots. Home
Assistant bundles go2rtc for its camera/WebRTC pipeline. Because go2rtc already understood
the *transport* for a related TUTK device family, extending it with a new source was the
most direct route to a maintainable, dependency-free integration.

---

## 3. Device Under Test

| Property | Value |
|---|---|
| Product | Dogness Smart Cam Treater T03 |
| Hardware model | T1020 |
| Firmware (reported) | V1.0.0 |
| Video codec | H.264 (TUTK codec id `0x4E`) |
| Resolution | 1280 × 720 |
| Transport | TUTK/Kalay P2P over UDP |
| LAN session port (observed) | high ephemeral port (e.g. `488xx`) |
| Cloud region endpoint | `apius.dogness.com:10443` |

---

## 4. Methodology

Analysis proceeded in five overlapping phases: application decompilation, native-library
disassembly, an instrumented native bridge, packet capture, and clean-room reimplementation.
The work was conducted with an Android handset connected over ADB for dynamic analysis and a
Windows workstation on the same LAN as the device.

### 4.1 Static analysis of the mobile application

The official Android application package (APK) was obtained and decompiled with:

- **`jadx`** — DEX-to-Java decompilation for readable high-level logic.
- **`apktool`** — resource and smali extraction for details lost by `jadx`.

A string- and path-mining script over the merged `classes*.dex` located the cloud API paths,
the P2P glue classes, and — critically — the application's camera controller
(`Camera.java`). From this layer we recovered:

- The **hard-coded AV account name** used for every device login.
- The **TUTK SDK license key** passed to the native layer (required to initialize the SDK;
  redacted here and not used by the final pure-Go implementation).
- The call sequence into the native SDK, and the IOCtrl type numbers used for device control.

### 4.2 Disassembly of the native libraries

TUTK functionality lives in vendor-supplied native libraries (ARM64 `.so` objects for the
IOTC and AV APIs). These were analysed with **Ghidra** (headless) to recover:

- The layout of the `avClientStartEx` input/output structures.
- The internal authentication state machine. This was decisive: a function in the AV login
  path (observed at an offset such as `FUN_00120af0`) sets an internal state field
  (offset `+0xe8`) to `3` on **authentication rejection** and returns error `-20009`. The
  TUTK public headers label `-20009` as `AV_ER_TIMEOUT`, which initially misled analysis
  into treating a *rejected credential* as a *network timeout*. Ghidra disambiguated the two.

### 4.3 Instrumented native bridge

To exercise the real protocol against the device before any clean-room code existed, a small
**C bridge** was cross-compiled with the **Android NDK (r27c, `aarch64-linux-android29-clang`)**
and executed on the connected handset, linking the vendor `.so` libraries. Its initialization
sequence, recovered and confirmed empirically, was:

```
TUTK_SDK_Set_License_Key(<license key>)
IOTC_Setup_Session_Alive_Timeout(10)
IOTC_Initialize2(<port in 10000..19999>)
RDT_Initialize()
avInitialize(128)
IOTC_Connect_ByUID(<uid>)
avClientStartEx(&in, &out)   // in: account="admin", password, channel 0,
                             //     timeout 20, resend 1, security_mode 0, auth_type 0
```

Two early failures were diagnosed here:

- `IOTC_Initialize2` returned `-1005` until (a) the license key was set first and (b) the
  local bind port was constrained to the SDK's expected `10000–19999` range.
- `avClientStartEx` returned `-20009`. Guided by §4.2, this was understood as auth
  rejection rather than timeout. Sweeping candidate account names showed the login succeeds
  (login result `0`) only with the account **`admin`**, matching the value hard-coded in
  `Camera.java`.

The working bridge established a session, started video, and **dispensed a treat**,
confirming the semantics before reimplementation.

### 4.4 Packet capture and transcode analysis

With a known-good bridge, its own UDP traffic was captured on-device with **PCAPdroid** and
decoded offline. (An attempt to capture the *official app's* traffic via an HTTPS-intercept
proxy was abandoned after the proxy configuration disrupted the app's networking; decoding
the bridge's own traffic proved cleaner and sufficient.) A Python reimplementation of the
TUTK **transcode** (§5.3) was written and verified **byte-exact** against the captures,
confirming our understanding of the obfuscation and of each packet's structure.

### 4.5 Clean-room reimplementation

Finally, the protocol was reimplemented in Go by extending go2rtc's `pkg/tutk`, with no
dependency on the vendor libraries. All device-specific behaviour was validated against the
physical device (§8).

---

## 5. Protocol Analysis

### 5.1 Layering

The device communicates entirely over **UDP**. A single socket carries the IOTC handshake,
then the AV session: periodic keep-alive "pings", control IOCtrls on a command channel, and
media frames (fragmented across UDP datagrams) on a data channel. All payloads are obfuscated
with the transcode of §5.3.

### 5.2 Protocol-version markers (the key difference)

Every datagram begins with a 4-byte framing header. The first three bytes are a magic:

```
byte 0..1 : 04 02      fixed
byte 2    : VV         protocol/version marker
byte 3    : 02         connection-stage indicator (handshake)
byte 4..5 : LEN        little-endian (len(datagram) - 16)
```

The decisive finding for interoperability was the value of the version marker **VV**:

- **The T03 client (our side) must send `0x1B` (27).**
- **The device replies with `0x19` (25).**

go2rtc's existing TUTK code was written for a dialect that sends `0x19`. go2rtc selects its
session implementation from the *observed* version byte (`>= 25` → "session-25" chunked
media), which remained correct for the T03's replies; but the *outgoing* magic, the reported
**SDK-version bytes**, and one handshake packet size had to be changed to the T03's values:

| Field | go2rtc default (Wyze dialect) | T03 value |
|---|---|---|
| Outgoing magic byte 2 | `0x19` | `0x1B` |
| SDK-version bytes (in handshake) | `06 00 03 03` | `03 07 04 03` |
| Broadcast/direct connect packet size | 68 bytes | 88 bytes |

With those three changes, the device's existing `session-25` logic in go2rtc handled media
and IOCtrls correctly.

### 5.3 The "Charlie" transcode

TUTK obfuscates datagrams with a keyed, block-wise, reversible byte transform — **not**
encryption when the session's `security_mode` is `0`, because the key is a fixed, public
constant. go2rtc names it after an in-source homage; the 32-byte key constant is the ASCII
string:

```
"Charlie is the designer of P2P!!"
```

Processing is per 16-byte block (the first 16 bytes of the key are the per-block XOR mask).
The decode (`ReverseTransCodePartial`) is, for each block:

1. For each of the four little-endian 32-bit words at offset `i ∈ {0,4,8,12}`, rotate left
   by `i + 3` bits.
2. Byte-swap the block.
3. XOR each byte `k` with `key[k]`.
4. For each 32-bit word at offset `i`, rotate left by `i + 1` bits.

Encoding is the inverse. A trailing partial block (`< 16` bytes) is handled with the
swap-and-XOR step only. Because the key is constant and `security_mode = 0`, this provides
**no confidentiality** against an on-path adversary — a point returned to in §9.

### 5.4 Session identifier

Each session is tagged with an 8-byte identifier. On this platform it is simply the client's
wall-clock time: an 8-byte little-endian nanosecond `UNIX` timestamp at connect time.

### 5.5 Connection handshake

`IOTC_Connect_ByUID` is realized as a small state machine. The relevant stages and their
3-byte command tags (placed at datagram offset 8) are:

| Stage | Command tag | Purpose |
|---|---|---|
| Broadcast | `01 06 21` | LAN search / initial connect probe |
| Direct | `01 06 21` (stage byte 2) | Establish the direct session |
| Get-remote-IP | `03 02 34` | Ask a ThroughTek master to resolve the device's public endpoint (WAN path) |
| Remote ack / ok | `02 04 33` / `04 04 33` | Complete the relayed/holepunched path |

The UID (20 ASCII characters) is written at offset 16 of each handshake datagram; the
8-byte session id and the 4-byte SDK-version are placed at fixed offsets later in the packet.

A successful device response to the broadcast/direct probe is identified by the tag
**`02 06 12`** appearing at **offset 8** of the (de-transcoded) reply. The reply is a
200-byte datagram; the device echoes the queried **UID starting at offset 16**, which allows
a search response to be matched to a specific device (used for discovery, §5.9).

Crucially, because the Home Assistant host and the feeder share a LAN, the **direct path
alone suffices** — no master-server lookup or NAT traversal is required. This both simplifies
the implementation and keeps all traffic on the local network.

### 5.6 AV authentication

After the IOTC session is up, the AV layer authenticates. The `avClientStartEx` input
structure (recovered in §4.2) carries, among its fields:

| Field | Value |
|---|---|
| struct size | 56 |
| session id | (from IOTC) |
| channel | 0 |
| timeout | 20 |
| account | **`admin`** |
| password | device AV key (from cloud, §6) |
| resend | 1 |
| `security_mode` | 0 |
| `auth_type` | 0 |

The **`admin`** account name is fixed firmware-side; any other value yields the `-20009`
authentication rejection described in §4.2–4.3. In the clean-room implementation, the
go2rtc handshake waits for a reply whose AV-login status field indicates success
(`0x14`/`0x21` acknowledgements) before proceeding.

### 5.7 Media framing

Media arrives as fragmented frames on the data channel. Datagram headers are 28 or 36 bytes
depending on a frame-type byte (start / continuation / end variants), carrying a frame
number, a per-frame packet index and count, and a payload length. The session-25 chunking
layer reassembles fragments, using a small **reorder buffer** to tolerate UDP reordering and
emitting acknowledgement/counter messages back to the device.

A per-frame **FrameInfo** trailer accompanies the assembled frame. Its salient fields:

| Offset | Field | Notes |
|---|---|---|
| 0 | codec id | `0x4E` = H.264 on this device |
| 2 | flags | bit set ⇒ key frame (video) |
| 5 | FPS | |
| 8..11 | timestamp | ~50000 units/frame at 20 fps |
| 16..19 | payload size | |
| 20..23 | frame number | |

The payload is H.264 in Annex-B byte-stream form; converting start-code delimiters to
length-prefixed AVCC and extracting the SPS/PPS from the first key frame is sufficient to
advertise the stream to downstream consumers.

### 5.8 Device-control IOCtrls

Control messages are typed 32-bit **IOCtrl** commands with a byte payload. Those relevant
here:

| IOCtrl (dec) | Hex | Meaning | Payload |
|---|---|---|---|
| 511 | `0x1FF` | `IPCAM_START` — begin streaming | 8 zero bytes |
| 816 | `0x330` | device-info request | small; device replies 817 |
| 8318 | `0x1FBE` | `TRANSFER_TTY_DATA_REQ` — **dispense a treat** | 32-byte TTY payload |

After IOCtrl **511** the device auto-streams video on the data channel. The 32-byte treat
payload is a fixed command block; the significant non-zero bytes are:

```
offset:  0  1  2  3  4  5  6  7  ...
bytes : 01 00 05 00 FF FF 0F 01  00 ...  (remaining bytes zero)
```

**Behavioural finding (non-obvious).** The device **ignores the treat IOCtrl unless a media
session is actively streaming.** Sending `8318` on an authenticated-but-idle session has no
physical effect; the command only dispenses when preceded by `511` *and* while frames are
actually being read. The final implementation therefore guarantees an active stream around
every dispense (reusing a live view if one exists, else opening a brief session, starting
video, dispensing, and closing).

### 5.9 LAN auto-discovery

The broadcast stage of §5.5 doubles as a **LAN device-search**. Sending a transcoded
broadcast connect probe for a given UID to the global broadcast address and to each
interface's directed broadcast on UDP port **32761** (the TUTK default LAN-listen port)
causes the matching device to reply from its **real session address** (`ip:port`). Because
the device echoes the UID at offset 16 of the reply (§5.5), the correct device is identified
unambiguously even if several TUTK devices are present. This yields the device's current LAN
address from the UID alone, with no configuration and automatic adaptation to DHCP changes.

> This search relies on L2 broadcast reaching the device, so the discovering process must
> share the LAN broadcast domain (see §7 on host networking).

---

## 6. Cloud Provisioning API

Although media and control are fully local, the device's **P2P UID** and **AV key** must be
obtained once. They are available from the Dogness account API, avoiding any need for the
user to extract them manually.

**Host.** `https://apius.dogness.com:10443` (US region). The endpoint presents a certificate
chain not anchored in the standard public root store (the app pins its own trust); the
provisioning client therefore cannot rely on default verification.

**Login.** `POST /service/authentication/login/password`, form-encoded, with app-identifying
headers (an app version, a channel, a synthetic terminal id, model/type). The password is
transmitted as **two** salted MD5 digests:

```
password = md5(plaintext + "tamboo")
passwd   = md5(plaintext + "7e8d69c3370767a9f3e8380f2b1784be")
```

(The salts are static application constants.) A successful response carries
`data.accessToken`.

**Device list.** `POST /service/api/device/permission/list_all_device_by_user` with the
token in an `x-auth-token` header returns the account's devices. Per device, the fields used
are:

| Field | Meaning |
|---|---|
| `devUid` | TUTK P2P UID (20 chars) |
| `password` | TUTK AV key for `avClientStartEx` |
| `devName` | user-facing name |
| `devModel` | e.g. `T1020` |

Notably the list **does not** include the device's LAN IP — hence the discovery mechanism of
§5.9. A header field `timeZone` is expressed in **whole hours** (not seconds); this tripped an
early implementation that assumed a seconds offset.

---

## 7. Implementation

### 7.1 Architecture

The implementation extends go2rtc with a self-contained `dogness://` source and a treat
endpoint, compiled into a single static binary with **no native dependencies**
(`CGO_ENABLED=0`). The chosen seam is deliberate: go2rtc already solves media transcoding and
republishing (RTSP/WebRTC/HLS/snapshots) and is the media engine Home Assistant itself uses.

Components added:

- **`pkg/tutk` patches** — the T03 magic/SDK-version/size of §5.2, plus two transport fixes
  found during bring-up (below).
- **`pkg/dogness/producer.go`** — a go2rtc `Producer`: dials the device, issues IOCtrl `511`,
  probes for the H.264 SPS to advertise the codec, then streams frames (Annex-B → AVCC) to
  subscribers. Exposes a `Feed()` method (IOCtrl `8318`) and a UID-keyed registry so the
  treat endpoint can reuse a live session.
- **`pkg/dogness/discover.go`** — the LAN broadcast search of §5.9, with a short address
  cache.
- **`pkg/dogness/cloud.go`** — the provisioning client of §6.
- **`internal/dogness/dogness.go`** — the go2rtc module: reads the account configuration,
  logs in, lists devices, registers a stream per device via `streams.New`, and serves
  `POST /api/dogness/feed`.

A stream source is registered as `dogness://discover?uid=…&username=admin&password=…`; the
literal host `discover` triggers §5.9 at connect time, so a changed DHCP lease is handled
transparently.

### 7.2 Transport fixes discovered during bring-up

Three defects surfaced only against hardware and are recorded for completeness:

1. **Address-family / comparison bug.** On the author's platform the socket produced
   16-byte IP representations while the peer address was 4-byte, so a naïve byte-equality
   check silently discarded every reply. Fixed by forcing IPv4 sockets and comparing with
   `net.IP.Equal`.
2. **Handshake read-deadline leaked into streaming.** The 5-second deadline used to bound the
   handshake was never cleared, so the media reader hit EOF ~5 s after connecting. Fixed by
   clearing the deadline before entering the read loop.
3. **No video without `511`.** The device does not stream until it receives
   `IPCAM_START`; sending it immediately after AV login is required.

### 7.3 Packaging as a Home Assistant add-on

The add-on ships a **prebuilt**, statically-linked go2rtc binary per architecture
(`aarch64`, `amd64`, `armv7`), so installation is near-instant with no on-device Go
toolchain or compile (an earlier build-from-source-at-install design proved too heavy for
low-power targets such as a Raspberry Pi). Each binary is produced by cloning upstream go2rtc
pinned to an exact commit, applying a small **overlay** of the changed/new files, and
cross-compiling (`CGO_ENABLED=0`); the overlay source and the reproducing build script are
published alongside the binaries, keeping the result auditable.

The add-on runs with **host networking**, which is required for the broadcast discovery of
§5.9 to reach the LAN (a default bridged container neither sees the LAN subnet nor forwards
L2 broadcasts). Configuration is reduced to the user's **Dogness account email and
password** (plus an optional timezone and stream name); UID, AV key, and IP are all derived
automatically. A start-up script renders the go2rtc configuration from the add-on options and
`exec`s the server.

---

## 8. Validation

The integration was validated against the physical device:

- **Video (clean-room, native platform).** go2rtc dialed the device via `dogness://`, probed
  H.264, and served RTSP; a downstream decoder retrieved a correct, timestamped 1280×720
  night-vision frame of the expected scene.
- **Treat (clean-room).** `POST /api/dogness/feed` returned success and a treat was physically
  dispensed (confirmed by the device owner), both while viewing and while idle (via the brief
  on-demand session of §5.8).
- **Cross-platform binary.** The Linux build connected to the device and started the producer
  from a Linux host, confirming the pure-Go path is not platform-specific.
- **Account-only provisioning.** Supplied only an account email and password, the add-on
  logged in, retrieved the device, auto-discovered its LAN address, and streamed live video —
  with no UID, key, or IP configured.
- **Reproducible build.** Building from a *pristine* upstream checkout plus the published
  overlay produced a working static binary, confirming the add-on's build path.

---

## 9. Security Analysis

The analysis surfaced several transport-security observations about the device (reported here
descriptively, not as an exploitation guide):

- **Obfuscation, not encryption.** With `security_mode = 0`, datagrams are protected only by
  the fixed, publicly known "Charlie" transcode (§5.3). An on-path adversary on the LAN can
  fully decode the control channel and the H.264 stream.
- **Fixed credentials.** The AV account name is a constant (`admin`); only the per-device key
  gates access. That key is retrievable from the account cloud API.
- **Known platform CVEs.** The underlying TUTK/Kalay platform has a documented history of
  session-layer vulnerabilities (e.g. CVE-2021-28372); devices that are reachable from the
  WAN via the master-server path inherit that exposure. Constraining the device to LAN-only
  operation — as this integration does — materially reduces attack surface.
- **Credential hygiene.** Because the cloud login returns the AV key, and because the device
  password may transit development logs during reverse engineering, rotating the account
  password after any such exposure is prudent. The published integration stores credentials
  only in the user's own Home Assistant instance and transmits them only to the official
  Dogness API.

For a privacy-sensitive deployment, blocking the feeder's outbound WAN access at the router
(while leaving LAN intact) is recommended; the integration continues to function because it
depends on no cloud media path.

---

## 10. Limitations and Future Work

- **Audio and two-way intercom** were not investigated; only H.264 video and the treat
  control were in scope. The TUTK AV layer supports audio channels, and go2rtc has the
  plumbing, so this is tractable future work.
- **Resolution/bitrate control** IOCtrls were not mapped; the device streams its default
  profile.
- **Region endpoints.** Only the US cloud host was used; other regions use different hosts.
- **Protocol-version coupling.** The implementation is validated for the T03's `0x1B`/`0x19`
  dialect and firmware V1.0.0; other Dogness models or future firmware may differ.
- **Discovery requires L2 reachability.** Segmented VLANs that block broadcast between Home
  Assistant and the feeder would require an explicit address or a discovery relay.

---

## 11. Reproducibility

The complete source of the integration — the go2rtc overlay, the Home Assistant add-on
packaging, and this report — is published under the MIT license. The add-on build pins the
exact upstream go2rtc commit it was validated against, so the binary is reproducible from
source. Device-, account-, and vendor-specific secrets are deliberately excluded; a
reproducing party supplies their own Dogness account, against their own hardware.

Tooling used: `jadx`, `apktool`, Ghidra (headless), Android NDK r27c, ADB, PCAPdroid, the Go
toolchain, `ffmpeg` (validation only), and go2rtc.

---

## 12. Conclusion

A camera treat-dispenser marketed as a cloud appliance was reduced, through static and
dynamic analysis, to a well-understood TUTK/Kalay device and then re-expressed as a local,
dependency-free Home Assistant integration. The essential findings were small but decisive:
the device's protocol-version markers (`0x1B`/`0x19`) and SDK-version bytes, the fixed `admin`
AV account and the misleading "timeout" that was really an auth rejection, the `511`/`8318`
control semantics with the active-stream precondition for dispensing, and a UID-based LAN
broadcast search that eliminates configuration. The result views the camera and drops a treat
entirely on the local network, with the vendor cloud used only once to read the owner's own
device key.

---

## Appendix A — Handshake datagram skeleton (broadcast/direct)

```
off  size  field
0     3    magic  04 02 1B          (client send; device replies 04 02 19)
3     1    stage  02
4     2    length little-endian = len - 16
8     3    command tag  01 06 21
16   20    UID (ASCII)
52    4    SDK version  03 07 04 03
56    8    session id (LE nanosecond timestamp)
64    1    stage byte (1 = broadcast, 2 = direct)
...        (zero padding to 88 bytes)
```

Device reply (200 bytes): tag `02 06 12` at offset 8; UID echoed at offset 16; version
marker byte at offset 2.

## Appendix B — Treat IOCtrl payload (32 bytes)

```
01 00 05 00 FF FF 0F 01 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
```

Sent as IOCtrl type `8318` (`0x1FBE`) on an actively streaming session.

## Appendix C — Glossary

| Term | Meaning |
|---|---|
| TUTK / Kalay | ThroughTek's P2P connectivity SDK family |
| IOTC | TUTK session/transport layer |
| AV | TUTK audio/video + auth + IOCtrl layer |
| IOCtrl | typed control message over an AV session |
| UID | 20-character device P2P identifier |
| AV key | per-device password for AV login |
| Annex-B / AVCC | two H.264 byte-stream framings |
| SPS / PPS | H.264 sequence/picture parameter sets |

---

*This report accompanies the `ha-dogness-t03` Home Assistant add-on. It documents
interoperability research performed by the author on hardware and accounts they own. "Dogness"
and "TUTK"/"Kalay" are trademarks of their respective owners; this work is independent and
unaffiliated.*
