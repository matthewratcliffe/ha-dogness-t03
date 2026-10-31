package dogness

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/dogness"
	"github.com/rs/zerolog"
)

// Config is the `dogness:` section of go2rtc.yaml.
type Config struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Timezone int    `yaml:"timezone"`
	Stream   string `yaml:"stream"` // optional name for a single feeder
}

// entry is a resolved feeder, keyed by stream name, for the treat endpoint.
type entry struct {
	uid      string
	password string
}

var (
	cfg     Config
	log     zerolog.Logger
	feedMu  sync.Mutex
	feeders = map[string]entry{} // stream name -> device
)

func Init() {
	var v struct {
		Cfg Config `yaml:"dogness"`
	}
	app.LoadConfig(&v)
	cfg = v.Cfg

	log = app.GetLogger("dogness")

	// dogness://<host|discover>?uid=..&username=admin&password=..
	streams.HandleFunc("dogness", func(rawURL string) (core.Producer, error) {
		log.Debug().Msgf("dial %s", rawURL)
		return dogness.NewProducer(rawURL)
	})

	// POST /api/dogness/feed?id=<stream>  -> dispense one treat
	api.HandleFunc("api/dogness/feed", apiFeed)

	if cfg.Username != "" && cfg.Password != "" {
		go setup()
	}
}

// setup logs into the Dogness cloud, lists the account's feeders and
// registers a stream + treat entry for each. Retries until the network
// (and cloud) are reachable.
func setup() {
	for attempt := 1; ; attempt++ {
		if err := trySetup(); err != nil {
			log.Warn().Err(err).Msgf("cloud setup attempt %d failed; retrying", attempt)
			time.Sleep(backoff(attempt))
			continue
		}
		return
	}
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 10 * time.Second
	if d > 2*time.Minute {
		d = 2 * time.Minute
	}
	return d
}

func trySetup() error {
	cloud := dogness.NewCloud(cfg.Timezone)
	if err := cloud.Login(cfg.Username, cfg.Password); err != nil {
		return err
	}

	devices, err := cloud.ListDevices()
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		return fmt.Errorf("no devices on account")
	}

	used := map[string]bool{}
	for i, d := range devices {
		name := streamName(d, i)
		for used[name] {
			name += "x"
		}
		used[name] = true

		q := url.Values{}
		q.Set("uid", d.UID)
		q.Set("username", "admin")
		q.Set("password", d.Password)
		src := "dogness://discover?" + q.Encode()

		if _, err := streams.New(name, src); err != nil {
			log.Warn().Err(err).Msgf("register stream %q", name)
			continue
		}

		feedMu.Lock()
		feeders[name] = entry{uid: d.UID, password: d.Password}
		feedMu.Unlock()

		log.Info().Msgf("feeder ready: stream=%q name=%q model=%s uid=%s", name, d.Name, d.Model, d.UID)
	}

	return nil
}

// streamName derives a stable go2rtc stream name for a feeder.
func streamName(d dogness.Device, index int) string {
	if cfg.Stream != "" && index == 0 {
		return cfg.Stream
	}
	s := slug(d.Name)
	if s == "" {
		s = "treater"
	}
	if index > 0 {
		s = fmt.Sprintf("%s-%d", s, index+1)
	}
	return s
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func apiFeed(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	feedMu.Lock()
	e, ok := feeders[id]
	// If no id given and exactly one feeder, use it.
	if !ok && id == "" && len(feeders) == 1 {
		for _, v := range feeders {
			e, ok = v, true
		}
	}
	feedMu.Unlock()

	if !ok {
		// Allow inline uid/password (e.g. manual testing).
		uid := r.URL.Query().Get("uid")
		if uid == "" {
			http.Error(w, "dogness: unknown feeder (set ?id=<stream>)", http.StatusNotFound)
			return
		}
		e = entry{uid: uid, password: r.URL.Query().Get("password")}
	}

	// host "" -> discover on LAN by uid
	if err := dogness.FeedByUID(e.uid, "", "admin", e.password); err != nil {
		log.Warn().Err(err).Msg("feed failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", api.MimeJSON)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
