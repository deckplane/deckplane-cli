package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultURL is the production Deckplane Cloud endpoint. Override per-call
// via Client.BaseURL when pointing at a dev or staging instance.
const DefaultURL = "https://cloud.deckplane.com"

// Client talks to Deckplane Cloud APIs. Zero value is ready to use; set
// BaseURL for dev/staging deployments.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New returns a client pointed at base (trailing slashes stripped) with a
// sensible default timeout. If base is empty, DefaultURL is used.
func New(base string) *Client {
	if base == "" {
		base = DefaultURL
	}
	return &Client{
		BaseURL:    strings.TrimRight(base, "/"),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// RegistryToken is the short-lived GHCR pull token Cloud mints on our behalf.
type RegistryToken struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	Username  string `json:"username"`
	Registry  string `json:"registry"`
}

// MintRegistryToken exchanges a license key for a 1-hour registry pull token.
// Errors distinguish common failure modes (invalid license, revoked,
// expired, Cloud unreachable) so callers can render friendlier messages.
func (c *Client) MintRegistryToken(licenseKey string) (*RegistryToken, error) {
	if licenseKey == "" {
		return nil, fmt.Errorf("license key is required")
	}

	body, _ := json.Marshal(map[string]string{"license_key": licenseKey})
	req, err := http.NewRequest(http.MethodPost,
		c.BaseURL+"/api/v1/licenses/registry-token",
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach Deckplane Cloud: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		var token RegistryToken
		if err := json.Unmarshal(raw, &token); err != nil {
			return nil, fmt.Errorf("malformed response from Cloud: %w", err)
		}
		return &token, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("license key rejected — check that you copied the full JWT")
	case http.StatusForbidden:
		return nil, fmt.Errorf("license is not currently valid: %s", extractError(raw))
	case http.StatusServiceUnavailable:
		return nil, fmt.Errorf("Cloud installer is temporarily disabled (%s)", extractError(raw))
	default:
		return nil, fmt.Errorf("unexpected Cloud response (HTTP %d): %s", resp.StatusCode, extractError(raw))
	}
}

func extractError(body []byte) string {
	var wrapper struct {
		Error string `json:"error"`
		Note  string `json:"note"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.Error != "" {
		if wrapper.Note != "" {
			return wrapper.Error + " — " + wrapper.Note
		}
		return wrapper.Error
	}
	return strings.TrimSpace(string(body))
}
