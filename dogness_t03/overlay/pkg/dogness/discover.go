package dogness

import (
	"bytes"
	"fmt"
	"net"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
)

// discoverPort is the TUTK/Kalay LAN-search port. Devices listen for a
// broadcast ConnectByUID here and reply from their real session address.
const discoverPort = 32761

// Discover finds a device's LAN address (ip:port) by its UID using a TUTK
// LAN broadcast search. Requires the process to share the LAN broadcast
// domain (host networking), otherwise the broadcast never reaches the device.
func Discover(uid string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 4 * time.Second
	}

	sid := tutk.GenSessionID()
	enc := tutk.TransCodePartial(nil, tutk.ConnectByUID(1 /*stageBroadcast*/, uid, sid))

	udp, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return "", err
	}
	defer udp.Close()

	found := make(chan string, 1)

	go func() {
		buf := make([]byte, 1500)
		uidb := []byte(uid)
		for {
			_ = udp.SetReadDeadline(time.Now().Add(timeout))
			n, addr, err := udp.ReadFromUDP(buf)
			if err != nil {
				return
			}
			dec := make([]byte, n)
			tutk.ReverseTransCodePartial(dec, buf[:n])
			// Device connect-ack: "\x02\x06\x12" at offset 8, UID echoed at 16.
			if n >= 36 && bytes.Index(dec, []byte("\x02\x06\x12")) == 8 &&
				bytes.HasPrefix(dec[16:], uidb) {
				select {
				case found <- fmt.Sprintf("%s:%d", addr.IP.String(), addr.Port):
				default:
				}
				return
			}
		}
	}()

	// Broadcast targets: global + each interface's directed broadcast.
	targets := []*net.UDPAddr{{IP: net.IPv4bcast, Port: discoverPort}}
	targets = append(targets, broadcastAddrs(discoverPort)...)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, t := range targets {
			_, _ = udp.WriteToUDP(enc, t)
		}
		select {
		case host := <-found:
			return host, nil
		case <-time.After(400 * time.Millisecond):
		}
	}

	return "", fmt.Errorf("dogness: device %s not found on LAN (needs host networking)", uid)
}

// broadcastAddrs returns the directed broadcast address of every IPv4
// interface that looks like a real LAN (skips loopback and down links).
func broadcastAddrs(port int) []*net.UDPAddr {
	var out []*net.UDPAddr
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 ||
			ifi.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := ifi.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			mask := ipnet.Mask
			bc := make(net.IP, 4)
			for i := 0; i < 4; i++ {
				bc[i] = ip4[i] | ^mask[i]
			}
			out = append(out, &net.UDPAddr{IP: bc, Port: port})
		}
	}
	return out
}
