package tray

import (
	_ "embed"
	"log"

	"github.com/getlantern/systray"
	"github.com/pkg/browser"

	"github.com/ibeckermayer/scroll4me/internal/app"
	"github.com/ibeckermayer/scroll4me/internal/config"
)

//go:embed icon.png
var iconBytes []byte

// OnReady returns a systray onReady callback that sets up the menu.
func OnReady(a *app.App) func() {
	return func() {
		// Set icon (template icon for macOS menu bar styling)
		systray.SetTemplateIcon(iconBytes, iconBytes)
		systray.SetTitle("")
		systray.SetTooltip("scroll4me - X digest without the doomscrolling")

		// Auth status (disabled, just for display)
		var authStatusLabel string
		if a.IsAuthenticated() {
			authStatusLabel = "● Connected to X"
		} else {
			authStatusLabel = "○ Not connected"
		}
		mAuthStatus := systray.AddMenuItem(authStatusLabel, "Authentication status")
		mAuthStatus.Disable()

		// Auth action (Login / Logout)
		var authActionLabel string
		if a.IsAuthenticated() {
			authActionLabel = "Logout"
		} else {
			authActionLabel = "Login to X"
		}
		mAuthAction := systray.AddMenuItem(authActionLabel, "Login or logout from X")

		systray.AddSeparator()

		// Generate Digest (combined scrape + analyze + build)
		mGenerateDigest := systray.AddMenuItem("Generate Digest", "Scrape, analyze, and create digest")

		systray.AddSeparator()

		// View last digest
		mViewDigest := systray.AddMenuItem("View Last Digest", "Open last digest file")

		// Edit config
		mEditConfig := systray.AddMenuItem("Edit Config", "Open config file in editor")

		// Reload config
		mReloadConfig := systray.AddMenuItem("Reload Config", "Reload configuration from disk")

		systray.AddSeparator()

		// Quit
		mQuit := systray.AddMenuItem("Quit", "Exit scroll4me")

		// Helper to update auth UI
		updateAuthUI := func() {
			if a.IsAuthenticated() {
				mAuthStatus.SetTitle("● Connected to X")
				mAuthAction.SetTitle("Logout")
			} else {
				mAuthStatus.SetTitle("○ Not connected")
				mAuthAction.SetTitle("Login to X")
			}
		}

		// Handle menu clicks
		go func() {
			for {
				select {
				case <-mAuthAction.ClickedCh:
					if a.IsAuthenticated() {
						if err := a.TriggerLogout(); err != nil {
							log.Printf("Logout error: %v", err)
						}
					} else {
						if err := a.TriggerLogin(); err != nil {
							log.Printf("Login error: %v", err)
						}
					}
					updateAuthUI()

				case <-mGenerateDigest.ClickedCh:
					go func() {
						if err := a.GenerateDigest(); err != nil {
							log.Printf("Generate digest error: %v", err)
						}
					}()

				case <-mViewDigest.ClickedCh:
					if err := a.ViewLastDigest(); err != nil {
						log.Printf("View digest error: %v", err)
					}

				case <-mEditConfig.ClickedCh:
					path, err := config.ConfigPath()
					if err != nil {
						log.Printf("Failed to get config path: %v", err)
						continue
					}
					if err := browser.OpenFile(path); err != nil {
						log.Printf("Failed to open config file: %v", err)
					}

				case <-mReloadConfig.ClickedCh:
					if err := a.ReloadConfig(); err != nil {
						log.Printf("Failed to reload config: %v", err)
					}

				case <-mQuit.ClickedCh:
					systray.Quit()
				}
			}
		}()
	}
}

// OnExit is the systray onExit callback.
func OnExit() {
	log.Println("scroll4me shutting down...")
}

