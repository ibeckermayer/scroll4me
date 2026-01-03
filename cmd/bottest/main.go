// Command bottest opens bot.sannysoft.com in a browser using the same
// stealth options as the main scraper, allowing you to audit the browser fingerprint.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/chromedp/chromedp"
	"github.com/ibeckermayer/scroll4me/internal/browser"
)

func main() {
	log.Println("Opening bot.sannysoft.com with stealth browser options...")
	log.Println("Close the browser window when done inspecting.")

	opts := browser.Options(false) // non-headless so you can see it

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	err := chromedp.Run(ctx,
		chromedp.Navigate("https://bot.sannysoft.com"),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	)
	if err != nil {
		log.Fatalf("Failed to navigate: %v", err)
	}

	fmt.Println("Press Enter to close the browser...")
	fmt.Scanln()

	log.Println("Done.")
}

