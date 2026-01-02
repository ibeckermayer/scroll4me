package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

// Manager handles X.com authentication
type Manager struct {
	cookieStore *CookieStore
}

// NewManager creates a new auth manager
func NewManager(cookieStore *CookieStore) *Manager {
	return &Manager{cookieStore: cookieStore}
}

// IsAuthenticated checks if we have valid stored credentials
func (m *Manager) IsAuthenticated() bool {
	return m.cookieStore.IsValid()
}

// Login opens a browser window for the user to log in to X.com
// Returns extracted cookies on successful login
func (m *Manager) Login(ctx context.Context) error {
	// Create a visible (headful) browser context
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), // Visible browser
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("start-maximized", true),
		// Prevent `navigator.webdriver = true`, which seems enough to trick
		// X into believing we're not using a browser automation tool.
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	browserCtx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Navigate to X login page
	err := chromedp.Run(browserCtx,
		chromedp.Navigate("https://x.com/login"),
	)
	if err != nil {
		return fmt.Errorf("failed to navigate to login page: %w", err)
	}

	// Wait for successful login by polling for indicators
	err = m.waitForLogin(browserCtx)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Extract cookies
	cookies, err := m.extractCookies(browserCtx)
	if err != nil {
		return fmt.Errorf("failed to extract cookies: %w", err)
	}

	// Save cookies
	if err := m.cookieStore.Save(cookies); err != nil {
		return fmt.Errorf("failed to save cookies: %w", err)
	}

	return nil
}

// waitForLogin polls until the user has successfully logged in
func (m *Manager) waitForLogin(ctx context.Context) error {
	timeout := time.After(5 * time.Minute) // Give user 5 minutes to log in
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("login timeout exceeded")
		case <-ticker.C:
			// Check if we're on the home page (indicates successful login)
			var url string
			err := chromedp.Run(ctx,
				chromedp.Location(&url),
			)
			if err != nil {
				continue
			}

			// Check URL for home page
			if url == "https://x.com/home" || url == "https://twitter.com/home" {
				// Additional check: verify auth_token cookie exists
				cookies, err := m.extractCookies(ctx)
				if err != nil {
					continue
				}
				for _, c := range cookies {
					if c.Name == "auth_token" && c.Value != "" {
						return nil // Successfully logged in
					}
				}
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// extractCookies gets all cookies from the browser
func (m *Manager) extractCookies(ctx context.Context) ([]*network.Cookie, error) {
	var cookies []*network.Cookie

	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = storage.GetCookies().Do(ctx)
			return err
		}),
	)

	return cookies, err
}

// Logout clears stored credentials
func (m *Manager) Logout() error {
	return m.cookieStore.Clear()
}

// GetCookies returns the stored cookies for use in scraping
func (m *Manager) GetCookies() ([]*network.Cookie, error) {
	return m.cookieStore.GetXCookies()
}
