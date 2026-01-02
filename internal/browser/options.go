// Package browser provides shared chromedp configuration with anti-bot-detection measures.
package browser

import "github.com/chromedp/chromedp"

// DefaultUserAgent is a realistic Chrome user agent
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// Options returns chromedp allocator options with anti-bot-detection measures.
// All browser instances should use this to ensure consistent stealth configuration.
func Options(headless bool) []chromedp.ExecAllocatorOption {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", headless),

		// Prevent navigator.webdriver = true detection
		// This is the most important flag - X.com checks this
		chromedp.Flag("disable-blink-features", "AutomationControlled"),

		// Use a realistic user agent
		chromedp.UserAgent(DefaultUserAgent),

		// Realistic window size
		chromedp.WindowSize(1920, 1080),

		// Disable automation-related extensions and features
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
	)

	if headless {
		opts = append(opts, chromedp.Flag("disable-gpu", true))
	}

	return opts
}
