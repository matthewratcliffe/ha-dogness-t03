#!/usr/bin/with-contenv bashio

USERNAME="$(bashio::config 'username')"
PASSWORD="$(bashio::config 'password')"
TZCFG="$(bashio::config 'timezone')"
STREAM="$(bashio::config 'stream')"
LOG_LEVEL="$(bashio::config 'log_level')"
MQTT_CAMERA="$(bashio::config 'mqtt_camera')"

[ -z "${LOG_LEVEL}" ] && LOG_LEVEL="info"
[ -z "${STREAM}" ] && STREAM="treater"

if [ -z "${USERNAME}" ] || [ -z "${PASSWORD}" ]; then
    bashio::log.fatal "Set your Dogness account 'username' and 'password' in the Configuration tab."
    exit 1
fi

# Auto-detect timezone hours from the system if left at 0.
if [ -z "${TZCFG}" ] || [ "${TZCFG}" = "0" ]; then
    Z="$(date +%z)"            # e.g. +1100 or -0500
    SIGN="${Z:0:1}"; HH="${Z:1:2}"
    TZCFG="$((10#${HH}))"
    [ "${SIGN}" = "-" ] && TZCFG="$((-TZCFG))"
fi

bashio::log.info "Dogness Treater starting (tz=${TZCFG}, stream=${STREAM})"

cat > /etc/go2rtc.yaml <<EOF
api:
  listen: ":1984"
rtsp:
  listen: ":8554"
webrtc:
  listen: ":8555"
log:
  level: ${LOG_LEVEL}
dogness:
  username: "${USERNAME}"
  password: "${PASSWORD}"
  timezone: ${TZCFG}
  stream: "${STREAM}"
EOF

# ---- start go2rtc (video + treat API) in the background ----
/usr/bin/go2rtc -config /etc/go2rtc.yaml &
GO2RTC_PID=$!

# =====================================================================
# MQTT discovery: ship a ready-made camera + treat button to Home
# Assistant, so they appear as entities with no YAML or manual setup.
# Requires the Mosquitto broker add-on (the "mqtt" service).
# =====================================================================
start_mqtt() {
    if ! bashio::services.available "mqtt"; then
        bashio::log.warning "No MQTT broker found. Install the 'Mosquitto broker' add-on to get the ready-made camera + treat button entities. (Video and the treat API still work.)"
        return
    fi

    local MH MP MU MPW
    MH="$(bashio::services mqtt 'host')"
    MP="$(bashio::services mqtt 'port')"
    MU="$(bashio::services mqtt 'username')"
    MPW="$(bashio::services mqtt 'password')"

    local BASE="dogness/${STREAM}"
    MQ=(-h "${MH}" -p "${MP}")
    [ -n "${MU}" ] && MQ+=(-u "${MU}" -P "${MPW}")

    # Connectivity check; under host networking the internal broker name may
    # not resolve, so fall back to localhost (Mosquitto exposes 1883 on host).
    if ! mosquitto_pub "${MQ[@]}" -r -t "${BASE}/status" -m "online" 2>/dev/null; then
        bashio::log.warning "MQTT host '${MH}' unreachable; trying 127.0.0.1"
        MH="127.0.0.1"
        MQ=(-h "${MH}" -p "${MP}")
        [ -n "${MU}" ] && MQ+=(-u "${MU}" -P "${MPW}")
        if ! mosquitto_pub "${MQ[@]}" -r -t "${BASE}/status" -m "online" 2>/dev/null; then
            bashio::log.error "Could not reach the MQTT broker; skipping ready-made entities."
            return
        fi
    fi
    bashio::log.info "MQTT connected (${MH}:${MP}); publishing camera + treat button"

    local DEV='{"identifiers":["dogness_'"${STREAM}"'"],"name":"Dogness Treater","manufacturer":"Dogness","model":"Smart Cam Treater T03"}'

    # Treat button
    mosquitto_pub "${MQ[@]}" -r \
        -t "homeassistant/button/dogness_${STREAM}/config" \
        -m '{"name":"Treat","unique_id":"dogness_'"${STREAM}"'_treat","command_topic":"'"${BASE}"'/treat/set","payload_press":"PRESS","icon":"mdi:bone","availability_topic":"'"${BASE}"'/status","device":'"${DEV}"'}'

    # Camera (snapshots pushed below)
    if bashio::var.true "${MQTT_CAMERA}"; then
        mosquitto_pub "${MQ[@]}" -r \
            -t "homeassistant/camera/dogness_${STREAM}/config" \
            -m '{"name":"Dog Treater","unique_id":"dogness_'"${STREAM}"'_cam","topic":"'"${BASE}"'/image","availability_topic":"'"${BASE}"'/status","device":'"${DEV}"'}'
    fi

    # Button command listener: any message on the command topic dispenses.
    (
        while true; do
            mosquitto_sub "${MQ[@]}" -t "${BASE}/treat/set" 2>/dev/null | while read -r _msg; do
                bashio::log.info "treat button pressed"
                curl -s -m 25 -X POST "http://127.0.0.1:1984/api/dogness/feed?id=${STREAM}" >/dev/null 2>&1 || true
            done
            sleep 3
        done
    ) &

    # Camera snapshot pipeline (optional). Uses go2rtc's own snapshot endpoint
    # (http://.../api/frame.jpeg) rather than an RTSP loopback — go2rtc's RTSP
    # path can be unreliable for this device while its internal/WebRTC path is
    # solid. A lightweight MJPEG consumer keeps the stream warm so each
    # snapshot is instant (and avoids reconnecting to the feeder every poll).
    if bashio::var.true "${MQTT_CAMERA}"; then
        ( while true; do
            curl -s -N -m 3600 "http://127.0.0.1:1984/api/stream.mjpeg?src=${STREAM}" >/dev/null 2>&1 || true
            sleep 2
          done ) &
        ( while true; do
            if curl -sf -m 10 -o /tmp/snap.jpg "http://127.0.0.1:1984/api/frame.jpeg?src=${STREAM}"; then
                mosquitto_pub "${MQ[@]}" -t "${BASE}/image" -f /tmp/snap.jpg 2>/dev/null || true
            fi
            sleep 2
          done ) &
    fi
}

# Give go2rtc a moment to log in to the cloud and register the stream.
( sleep 8; start_mqtt ) &

# Container lives as long as go2rtc does.
wait "${GO2RTC_PID}"
