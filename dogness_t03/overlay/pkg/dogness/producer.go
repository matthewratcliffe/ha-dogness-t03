// Package dogness implements a go2rtc source for Dogness TUTK/Kalay treat
// dispenser cameras (e.g. Smart Cam Treater T03 / T1020).
//
// URL format:
//
//	dogness://<ip>:<port>?uid=<UID>&username=admin&password=<TUTK password>
//
// The device speaks the TUTK/Kalay P2P protocol (handled by pkg/tutk). It
// streams H.264 video after an IOTYPE_USER_IPCAM_START (511) control message,
// and dispenses a treat on an IOTYPE_USER_IPCAM_TRANSFER_TTY_DATA_REQ (8318)
// control message carrying the feed payload.
package dogness

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/tutk"
	"github.com/pion/rtp"
)

const (
	// ctrlStart starts the video stream (IOTYPE_USER_IPCAM_START).
	ctrlStart uint32 = 511
	// ctrlFeed dispenses a treat (IOTYPE_USER_IPCAM_TRANSFER_TTY_DATA_REQ).
	ctrlFeed uint32 = 8318
)

// feedPayload is the 32-byte TTY payload that triggers one treat drop.
// Recovered by reverse-engineering the official app / native bridge.
func feedPayload() []byte {
	p := make([]byte, 32)
	p[0] = 0x01
	p[2] = 0x05
	p[4] = 0xFF
	p[5] = 0xFF
	p[6] = 0x0F
	p[7] = 0x01
	return p
}

// registry tracks live producers by UID so the treat API can reuse an
// already-connected session instead of opening a second one.
var (
	regMu    sync.Mutex
	registry = map[string]*Producer{}
)

// hostCache remembers the last discovered LAN address per UID (short TTL),
// so repeated connects don't re-broadcast every time.
var (
	hostMu    sync.Mutex
	hostCache = map[string]hostEntry{}
)

type hostEntry struct {
	host string
	when time.Time
}

const hostTTL = 60 * time.Second

// resolveHost returns host if it already points at a device (ip:port);
// otherwise it discovers the device on the LAN by UID.
func resolveHost(host, uid string) (string, error) {
	if host != "" && host != "discover" && host != "auto" {
		return host, nil
	}
	if uid == "" {
		return "", fmt.Errorf("dogness: need uid to discover device")
	}

	hostMu.Lock()
	if e, ok := hostCache[uid]; ok && time.Since(e.when) < hostTTL {
		hostMu.Unlock()
		return e.host, nil
	}
	hostMu.Unlock()

	found, err := Discover(uid, 5*time.Second)
	if err != nil {
		return "", err
	}

	hostMu.Lock()
	hostCache[uid] = hostEntry{host: found, when: time.Now()}
	hostMu.Unlock()

	return found, nil
}

type Producer struct {
	core.Connection
	conn *tutk.Conn
	uid  string

	host     string
	username string
	password string
}

func NewProducer(rawURL string) (*Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	uid := query.Get("uid")
	username := query.Get("username")
	if username == "" {
		username = "admin"
	}
	password := query.Get("password")

	host := u.Host
	if host == "" {
		host = query.Get("host")
	}

	host, err = resolveHost(host, uid)
	if err != nil {
		return nil, err
	}

	conn, err := tutk.Dial(host, uid, username, password)
	if err != nil {
		return nil, fmt.Errorf("dogness: dial: %w", err)
	}

	// Ask the device to start streaming video.
	if err = conn.WriteCommand(ctrlStart, make([]byte, 8)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("dogness: start: %w", err)
	}

	prod := &Producer{
		conn:     conn,
		uid:      uid,
		host:     host,
		username: username,
		password: password,
	}

	medias, err := prod.probe()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	prod.Connection = core.Connection{
		ID:         core.NewID(),
		FormatName: "dogness",
		Protocol:   conn.Protocol(),
		RemoteAddr: conn.RemoteAddr().String(),
		Source:     rawURL,
		Medias:     medias,
		Transport:  conn,
	}

	if uid != "" {
		regMu.Lock()
		registry[uid] = prod
		regMu.Unlock()
	}

	return prod, nil
}

// probe reads frames until it finds an H.264 SPS so it can advertise the codec.
func (p *Producer) probe() ([]*core.Media, error) {
	_ = p.conn.SetDeadline(time.Now().Add(core.ProbeTimeout))

	var vcodec *core.Codec
	for vcodec == nil {
		_, frameData, err := p.conn.ReadPacket()
		if err != nil {
			return nil, fmt.Errorf("dogness: probe: %w", err)
		}
		if len(frameData) < 5 {
			continue
		}

		buf := annexb.EncodeToAVCC(frameData)
		if len(buf) >= 5 && h264.NALUType(buf) == h264.NALUTypeSPS {
			vcodec = h264.AVCCToCodec(buf)
		}
	}

	_ = p.conn.SetDeadline(time.Time{})

	return []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{vcodec},
		},
	}, nil
}

func (p *Producer) Start() error {
	for {
		_ = p.conn.SetDeadline(time.Now().Add(core.ConnDeadline))

		_, frameData, err := p.conn.ReadPacket()
		if err != nil {
			return err
		}
		if len(frameData) == 0 {
			continue
		}

		pkt := &core.Packet{
			Header:  rtp.Header{},
			Payload: annexb.EncodeToAVCC(frameData),
		}

		for _, recv := range p.Receivers {
			if recv.Codec.Name == core.CodecH264 {
				recv.WriteRTP(pkt)
				break
			}
		}
	}
}

func (p *Producer) Stop() error {
	if p.uid != "" {
		regMu.Lock()
		if registry[p.uid] == p {
			delete(registry, p.uid)
		}
		regMu.Unlock()
	}
	return p.Connection.Stop()
}

// Feed dispenses one treat over this producer's live session.
func (p *Producer) Feed() error {
	pl := feedPayload()
	// The device wants the command a few times while the stream is active.
	var err error
	for i := 0; i < 3; i++ {
		if err = p.conn.WriteCommand(ctrlFeed, pl); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

// FeedByUID dispenses a treat for the given UID. If a producer is already
// streaming that device, it reuses that session; otherwise it opens a short
// ephemeral session (dial + start video + feed + close), which the device
// requires in order to accept the feed command.
func FeedByUID(uid, host, username, password string) error {
	regMu.Lock()
	prod := registry[uid]
	regMu.Unlock()

	if prod != nil {
		return prod.Feed()
	}

	if username == "" {
		username = "admin"
	}

	host, err := resolveHost(host, uid)
	if err != nil {
		return err
	}

	conn, err := tutk.Dial(host, uid, username, password)
	if err != nil {
		return fmt.Errorf("dogness: feed dial: %w", err)
	}
	defer conn.Close()

	if err = conn.WriteCommand(ctrlStart, make([]byte, 8)); err != nil {
		return fmt.Errorf("dogness: feed start: %w", err)
	}

	// Drain a couple of frames so the device considers the session active.
	go func() {
		for i := 0; i < 50; i++ {
			if _, _, e := conn.ReadPacket(); e != nil {
				return
			}
		}
	}()

	time.Sleep(1500 * time.Millisecond)

	pl := feedPayload()
	for i := 0; i < 3; i++ {
		if err = conn.WriteCommand(ctrlFeed, pl); err != nil {
			return fmt.Errorf("dogness: feed: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	return nil
}
