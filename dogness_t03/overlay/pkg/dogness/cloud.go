package dogness

import (
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// cloudHost is the Dogness app API (US region).
const cloudHost = "https://apius.dogness.com:10443"

// Salts recovered from the official app for the two password hashes.
const (
	saltPassword = "tamboo"
	saltPasswd   = "7e8d69c3370767a9f3e8380f2b1784be"
)

// Device is one feeder/camera from the account's device list.
type Device struct {
	UID      string // TUTK P2P UID
	Password string // TUTK AV password
	Name     string // user-given name, e.g. "Smart Cam Treater T03"
	Model    string // e.g. "T1020"
}

// Cloud is a minimal Dogness app-API client.
type Cloud struct {
	client   *http.Client
	token    string
	timezone int // hours offset, e.g. 11
}

func NewCloud(timezoneHours int) *Cloud {
	return &Cloud{
		// The Dogness API endpoint uses a chain not in the standard root
		// store; this is the user's own account login to a fixed vendor host.
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		},
		timezone: timezoneHours,
	}
}

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (c *Cloud) headers() map[string]string {
	tz := strconv.Itoa(c.timezone)
	tid := md5hex("ha-dogness-go2rtc")
	h := map[string]string{
		"x-lang":         "en",
		"lang":           "en",
		"timezone":       tz,
		"app-version":    "328",
		"channel":        "google",
		"terminalId":     tid,
		"terminalId0910": tid,
		"terminalModel":  "Pixel 7",
		"terminalType":   "phone",
	}
	if c.token != "" {
		h["x-auth-token"] = c.token
	}
	return h
}

func (c *Cloud) post(path string, form url.Values, out any) error {
	req, err := http.NewRequest("POST", cloudHost+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range c.headers() {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dogness cloud: %s -> HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Login authenticates and stores the access token.
func (c *Cloud) Login(username, password string) error {
	form := url.Values{
		"username": {username},
		"password": {md5hex(password + saltPassword)},
		"passwd":   {md5hex(password + saltPasswd)},
		"timeZone": {strconv.Itoa(c.timezone)},
	}

	var res struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			AccessToken string `json:"accessToken"`
		} `json:"data"`
	}
	if err := c.post("/service/authentication/login/password", form, &res); err != nil {
		return err
	}
	if res.Data.AccessToken == "" {
		return fmt.Errorf("dogness cloud: login failed: %s", res.Msg)
	}
	c.token = res.Data.AccessToken
	return nil
}

// ListDevices returns all feeders/cameras on the account.
func (c *Cloud) ListDevices() ([]Device, error) {
	var res struct {
		Data []struct {
			DevUID   string `json:"devUid"`
			Password string `json:"password"`
			DevName  string `json:"devName"`
			DevModel string `json:"devModel"`
		} `json:"data"`
	}
	if err := c.post("/service/api/device/permission/list_all_device_by_user", url.Values{}, &res); err != nil {
		return nil, err
	}

	var devices []Device
	for _, d := range res.Data {
		if d.DevUID == "" {
			continue
		}
		devices = append(devices, Device{
			UID:      d.DevUID,
			Password: d.Password,
			Name:     d.DevName,
			Model:    d.DevModel,
		})
	}
	return devices, nil
}
