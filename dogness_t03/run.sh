#!/usr/bin/with-contenv bashio
set -e

USERNAME="$(bashio::config 'username')"
PASSWORD="$(bashio::config 'password')"
TZCFG="$(bashio::config 'timezone')"
STREAM="$(bashio::config 'stream')"
LOG_LEVEL="$(bashio::config 'log_level')"

[ -z "${LOG_LEVEL}" ] && LOG_LEVEL="info"
[ -z "${STREAM}" ] && STREAM="treater"

if [ -z "${USERNAME}" ] || [ -z "${PASSWORD}" ]; then
    bashio::log.fatal "Set your Dogness account 'username' and 'password' in the add-on Configuration tab."
    exit 1
fi

# Auto-detect timezone hours from the system if left at 0.
if [ -z "${TZCFG}" ] || [ "${TZCFG}" = "0" ]; then
    Z="$(date +%z)"            # e.g. +1100 or -0500
    SIGN="${Z:0:1}"
    HH="${Z:1:2}"
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

exec /usr/bin/go2rtc -config /etc/go2rtc.yaml
