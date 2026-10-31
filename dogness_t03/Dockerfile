ARG BUILD_FROM
FROM ${BUILD_FROM}

# BUILD_ARCH is provided by the Home Assistant builder (aarch64|amd64|armv7).
ARG BUILD_ARCH

# Prebuilt, statically-linked (CGO disabled) pure-Go go2rtc with the Dogness
# TUTK source + treat API compiled in. No native libraries, no on-device
# compile (installs in seconds). Built from ./overlay applied to upstream
# go2rtc @ 7c05066c90ab6ad1fccb506d5f0408aa31ebe60f (v1.9.15); rebuild with
# ./scripts/build.sh.
COPY bin/go2rtc-${BUILD_ARCH} /usr/bin/go2rtc
RUN chmod a+x /usr/bin/go2rtc

COPY run.sh /run.sh
RUN chmod a+x /run.sh

CMD [ "/run.sh" ]
