ARG BUILD_FROM
FROM ${BUILD_FROM}

# BUILD_ARCH is provided by the Home Assistant builder (aarch64|amd64|armv7).
ARG BUILD_ARCH

# Prebuilt, statically-linked (CGO disabled) pure-Go go2rtc with the Dogness
# TUTK source + treat API compiled in. No native libraries, no on-device
# compile (installs in seconds). Built from ./overlay applied to upstream
# go2rtc @ 7c05066c90ab6ad1fccb506d5f0408aa31ebe60f (v1.9.15); rebuild with
# ./scripts/build.sh.
# ffmpeg: JPEG snapshots for the MQTT camera. mosquitto-clients + curl: MQTT
# discovery so the camera + treat button appear in HA automatically.
RUN apk add --no-cache ffmpeg mosquitto-clients curl

COPY bin/go2rtc-${BUILD_ARCH} /usr/bin/go2rtc
RUN chmod a+x /usr/bin/go2rtc

# Bundled self-contained Lovelace card (go2rtc player + wrapper); run.sh copies
# it into HA's www so it's served at /local/dogness-card.js.
COPY www/dogness-card.js /opt/dogness-card.js

COPY run.sh /run.sh
RUN chmod a+x /run.sh

CMD [ "/run.sh" ]
