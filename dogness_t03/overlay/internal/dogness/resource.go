package dogness

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// currentCardURL returns the Lovelace resource URL for the bundled card with a
// content hash query, so that whenever the card file changes the URL changes
// and browsers/apps re-fetch it instead of serving a stale cached copy.
func currentCardURL() string {
	for _, p := range []string{"/homeassistant/www/dogness-card.js", "/config/www/dogness-card.js"} {
		if b, err := os.ReadFile(p); err == nil {
			h := sha256.Sum256(b)
			return "/local/dogness-card.js?v=" + hex.EncodeToString(h[:4])
		}
	}
	return "/local/dogness-card.js"
}

// registerCard best-effort registers the bundled Lovelace card as a frontend
// resource via Home Assistant's WebSocket API (through the Supervisor proxy),
// so a non-technical user never has to add a resource or a YAML card — the
// "Dogness Live" card just appears in the dashboard card picker. It also
// removes stale (differently-versioned) copies so updates always take effect.
//
// Any failure is non-fatal and logged at debug.
func registerCard() {
	token := os.Getenv("SUPERVISOR_TOKEN")
	if token == "" {
		return // not running under the Supervisor
	}
	for attempt := 0; attempt < 30; attempt++ {
		if tryRegisterCard(token) {
			return
		}
		time.Sleep(10 * time.Second)
	}
}

func tryRegisterCard(token string) (done bool) {
	defer func() { _ = recover() }() // never crash the add-on over this

	want := currentCardURL()

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	c, _, err := dialer.Dial("ws://supervisor/core/websocket", nil)
	if err != nil {
		log.Debug().Err(err).Msg("card: ws dial failed")
		return false
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(15 * time.Second))

	read := func() map[string]any {
		var m map[string]any
		if c.ReadJSON(&m) != nil {
			return nil
		}
		return m
	}
	send := func(v map[string]any) bool { return c.WriteJSON(v) == nil }

	// auth handshake
	if m := read(); m == nil || m["type"] != "auth_required" {
		return false
	}
	if !send(map[string]any{"type": "auth", "access_token": token}) {
		return false
	}
	if m := read(); m == nil || m["type"] != "auth_ok" {
		log.Debug().Msg("card: ws auth not ok")
		return false
	}

	// list resources (dashboards must be in storage mode)
	if !send(map[string]any{"id": 1, "type": "lovelace/resources"}) {
		return false
	}
	m := read()
	if m == nil || m["success"] != true {
		log.Debug().Msg("card: cannot list resources (yaml-mode dashboards?)")
		return false
	}

	// Find existing dogness-card resources: keep an exact match, delete others.
	exists := false
	var stale []string
	if items, ok := m["result"].([]any); ok {
		for _, it := range items {
			r, ok := it.(map[string]any)
			if !ok {
				continue
			}
			url, _ := r["url"].(string)
			if !strings.HasPrefix(url, "/local/dogness-card.js") {
				continue
			}
			if url == want {
				exists = true
			} else if id, _ := r["id"].(string); id != "" {
				stale = append(stale, id)
			}
		}
	}

	nextID := 2
	for _, id := range stale {
		_ = send(map[string]any{"id": nextID, "type": "lovelace/resources/delete", "resource_id": id})
		_ = read()
		nextID++
		log.Info().Str("id", id).Msg("card: removed stale resource")
	}

	if exists {
		log.Info().Str("url", want).Msg("card: Lovelace resource up to date (Dogness Live)")
		return true
	}

	if !send(map[string]any{
		"id": nextID, "type": "lovelace/resources/create",
		"res_type": "module", "url": want,
	}) {
		return false
	}
	if m := read(); m != nil && m["success"] == true {
		log.Info().Str("url", want).Msg("card: registered Lovelace resource — add the 'Dogness Live' card from the picker")
		return true
	}
	return false
}
