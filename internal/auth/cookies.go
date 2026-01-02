package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/ibeckermayer/scroll4me/internal/config"
)

// CookieStore handles secure storage of X.com session cookies
type CookieStore struct {
	path string
}

// StoredCookies represents the persisted cookie data
type StoredCookies struct {
	Cookies    []*network.Cookie `json:"cookies"`
	CapturedAt time.Time         `json:"captured_at"`
	ExpiresAt  time.Time         `json:"expires_at"`
}

// NewCookieStore creates a cookie store at the given path
func NewCookieStore(path string) *CookieStore {
	return &CookieStore{path: path}
}

// DefaultCookieStorePath returns the default path for cookie storage
func DefaultCookieStorePath() (string, error) {
	configDir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "cookies.json"), nil
}

// Save persists cookies to disk
// TODO: Encrypt cookies at rest
func (cs *CookieStore) Save(cookies []*network.Cookie) error {
	dir := filepath.Dir(cs.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Find the earliest expiration among auth-related cookies
	var earliestExpiry time.Time
	for _, c := range cookies {
		if c.Name == "auth_token" || c.Name == "ct0" {
			exp := time.Unix(int64(c.Expires), 0)
			if earliestExpiry.IsZero() || exp.Before(earliestExpiry) {
				earliestExpiry = exp
			}
		}
	}

	stored := StoredCookies{
		Cookies:    cookies,
		CapturedAt: time.Now(),
		ExpiresAt:  earliestExpiry,
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cs.path, data, 0600)
}

// Load retrieves cookies from disk
func (cs *CookieStore) Load() (*StoredCookies, error) {
	data, err := os.ReadFile(cs.path)
	if err != nil {
		return nil, err
	}

	var stored StoredCookies
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}

	return &stored, nil
}

// IsValid checks if stored cookies are still valid
func (cs *CookieStore) IsValid() bool {
	stored, err := cs.Load()
	if err != nil {
		return false
	}

	// Check if cookies have expired
	if time.Now().After(stored.ExpiresAt) {
		return false
	}

	// Check for required cookies
	hasAuthToken := false
	hasCT0 := false
	for _, c := range stored.Cookies {
		if c.Name == "auth_token" {
			hasAuthToken = true
		}
		if c.Name == "ct0" {
			hasCT0 = true
		}
	}

	return hasAuthToken && hasCT0
}

// Clear removes stored cookies
func (cs *CookieStore) Clear() error {
	return os.Remove(cs.path)
}

// GetXCookies returns only the x.com related cookies for use in requests
func (cs *CookieStore) GetXCookies() ([]*network.Cookie, error) {
	stored, err := cs.Load()
	if err != nil {
		return nil, err
	}

	var xCookies []*network.Cookie
	for _, c := range stored.Cookies {
		if c.Domain == ".x.com" || c.Domain == "x.com" {
			xCookies = append(xCookies, c)
		}
	}

	return xCookies, nil
}
