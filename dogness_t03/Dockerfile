ARG BUILD_FROM

# ---- build stage: compile the patched go2rtc from source ----
# golang:alpine is multi-arch, so this builds natively for the target board.
FROM golang:1.27-alpine AS build

RUN apk add --no-cache git

# Upstream go2rtc, pinned to the exact commit this add-on was built against.
ARG GO2RTC_REF=7c05066c90ab6ad1fccb506d5f0408aa31ebe60f
RUN git clone https://github.com/AlexxIT/go2rtc /src \
    && cd /src \
    && git checkout "${GO2RTC_REF}"

# Overlay: the Dogness source (new files) + small TUTK/main patches.
COPY overlay/ /src/

WORKDIR /src
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /go2rtc .

# ---- run stage: Home Assistant base image ----
FROM ${BUILD_FROM}

COPY --from=build /go2rtc /usr/bin/go2rtc
COPY run.sh /run.sh
RUN chmod a+x /run.sh /usr/bin/go2rtc

CMD [ "/run.sh" ]
