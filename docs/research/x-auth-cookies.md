# X (Twitter) Authentication & Cookie Research

## Overview

This document tracks research on extracting authentication cookies from X.com for use in automated scraping.

## X.com Cookies Observed

From browser inspection (January 2026):

| Name                 | Domain           | HttpOnly | Secure | SameSite | Purpose                              |
| -------------------- | ---------------- | -------- | ------ | -------- | ------------------------------------ |
| `auth_token`         | .x.com           | ✓        | ✓      | None     | **Primary session token**            |
| `ct0`                | .x.com           | ✗        | ✓      | Lax      | CSRF token (needed for API requests) |
| `twid`               | .x.com           | ✗        | ✓      | None     | User ID (URL-encoded)                |
| `att`                | .x.com           | ✓        | ✓      | None     | Unknown auth-related                 |
| `kdt`                | .x.com           | ✓        | ✓      | -        | Unknown auth-related                 |
| `guest_id`           | .x.com           | ✗        | ✓      | None     | Guest identifier                     |
| `guest_id_ads`       | .x.com           | ✗        | ✓      | None     | Ads tracking                         |
| `guest_id_marketing` | .x.com           | ✗        | ✓      | None     | Marketing tracking                   |
| `personalization_id` | .x.com           | ✗        | ✓      | None     | Personalization                      |
| `d_prefs`            | .x.com           | ✗        | ✓      | Lax      | Display preferences                  |
| `lang`               | x.com            | ✗        | ✗      | -        | Language preference                  |
| `g_state`            | .x.com           | ✗        | ✗      | -        | Unknown state                        |
| `gt`                 | .x.com           | ✗        | ✗      | ✓        | Guest token?                         |
| `__cf_bm`            | .x.com           | ✓        | ✓      | -        | Cloudflare bot management            |
| `__cuid`             | .x.com           | ✗        | ✗      | Lax      | Unknown tracking                     |
| `IDE`                | .doubleclick.net | ✓        | ✓      | None     | Google ads                           |

### Critical Cookies for Authentication

The minimum required cookies for authenticated requests appear to be:

1. **`auth_token`** - Session authentication (HttpOnly)
2. **`ct0`** - CSRF token (must match `x-csrf-token` header)
3. **`twid`** - User identifier

The `auth_token` being HttpOnly is the main challenge - it cannot be accessed via JavaScript.

## Wails WebView Cookie Access

### Current Status (as of Jan 2026)

**Wails does NOT currently expose native cookie manager APIs.**

Relevant GitHub issues:

- [#3609 - Enable cookies in WebViews](https://github.com/wailsapp/wails/issues/3609)
- [#3908 - Cookies Support](https://github.com/wailsapp/wails/issues/3908)
- [#2590 - Cookie issues on macOS](https://github.com/wailsapp/wails/issues/2590)

### Why HttpOnly Matters

- **JavaScript (in browser)**: Cannot access HttpOnly cookies (by design - XSS protection)
- **Native WebView APIs**: CAN access HttpOnly cookies (WebView2's `CookieManager`, WKWebView's `WKHTTPCookieStore`)
- **Wails**: Does not currently expose these native APIs to Go code

### Native APIs That Could Work (if exposed)

| Platform | API                                   | Method              |
| -------- | ------------------------------------- | ------------------- |
| Windows  | WebView2 `ICoreWebView2CookieManager` | `GetCookiesAsync()` |
| macOS    | `WKHTTPCookieStore`                   | `getAllCookies()`   |
| Linux    | WebKitGTK `WebKitCookieManager`       | `get_cookies()`     |

## chromedp Alternative

**chromedp CAN access all cookies including HttpOnly** via Chrome DevTools Protocol.

### Methods

```go
// Get cookies for current page
cookies, err := network.GetCookies().Do(ctx)

// Get ALL cookies from browser
cookies, err := network.GetAllCookies().Do(ctx)
```

### Cookie struct includes:

- `Name`, `Value`, `Domain`, `Path`
- `HTTPOnly` (bool)
- `Secure`, `SameSite`, `Expires`

Sources:

- [chromedp cookie example](https://github.com/chromedp/examples/blob/master/cookie/main.go)
- [Get all cookies gist](https://gist.github.com/coffee-mug/37f62e1a788fde25c76ce2b691d6ce0f)

## Recommended Approach

Given Wails' current limitations, use a **hybrid approach**:

```
┌─────────────────────────────────────────────────────────────┐
│                    scroll4me Architecture                    │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Wails App (UI)                                             │
│  ├── Settings configuration                                 │
│  ├── Digest viewer                                          │
│  └── "Login to X" button                                    │
│           │                                                  │
│           ▼                                                  │
│  chromedp (headful) ─── Login Flow                          │
│  ├── Opens visible Chrome window                            │
│  ├── User logs in manually (handles 2FA)                    │
│  ├── Detect successful login (URL change or element)        │
│  └── Extract ALL cookies via network.GetAllCookies()        │
│           │                                                  │
│           ▼                                                  │
│  Secure Cookie Storage (encrypted file or OS keychain)      │
│           │                                                  │
│           ▼                                                  │
│  chromedp (headless) ─── Scraping                           │
│  ├── Load cookies into browser                              │
│  ├── Navigate to x.com/home (For You feed)                  │
│  ├── Scroll and extract posts                               │
│  └── Return structured data                                 │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Why This Works

1. **chromedp headful mode** - User sees a real Chrome window, can handle 2FA, CAPTCHAs, etc.
2. **DevTools Protocol** - Full cookie access including HttpOnly
3. **Wails for UI** - Clean settings interface, digest viewing
4. **Separation of concerns** - Wails doesn't need cookie access

### Tradeoffs

| Approach               | Pros                            | Cons                           |
| ---------------------- | ------------------------------- | ------------------------------ |
| Wails WebView login    | Single window, native feel      | Can't extract HttpOnly cookies |
| chromedp headful login | Full cookie access, handles 2FA | Opens separate Chrome window   |
| Manual cookie export   | No code needed                  | Terrible UX, cookies expire    |

## Cookie Expiration & Refresh

From observed data:

- `auth_token`: ~50 days
- `ct0`: ~163 days
- Most others: 21-67 days

Will need to handle:

1. Detecting expired sessions (401/403 responses, redirect to login)
2. Prompting user to re-authenticate
3. Possibly implementing session refresh if X supports it

## Security Considerations

1. **Cookie storage**: Should encrypt at rest (OS keychain preferred)
2. **auth_token scope**: Full account access - treat as password equivalent
3. **Rate limiting**: X may detect automated access, need to be gentle
4. **ToS**: Scraping may violate X terms of service - personal use only

## Open Questions

- [ ] Does X rotate `auth_token` on each session, or is it stable?
- [ ] What's the minimum cookie set needed for authenticated feed access?
- [ ] Does X have bot detection that will block chromedp?
- [ ] Can we use X's internal GraphQL API directly with cookies?

## References

- [WebView2 Cookie docs](https://learn.microsoft.com/en-us/microsoft-edge/webview2/reference/win32/icorewebview2cookie)
- [WKHTTPCookieStore docs](https://developer.apple.com/documentation/webkit/wkhttpcookiestore)
- [chromedp examples](https://github.com/chromedp/examples)
- [Wails cookie issues](https://github.com/wailsapp/wails/issues/3609)
