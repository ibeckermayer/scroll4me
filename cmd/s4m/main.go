// Command s4m is a dev CLI for scroll4me maintenance and debugging tasks.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/chromedp/chromedp"
	"github.com/pkg/browser"

	browseropts "github.com/ibeckermayer/scroll4me/internal/browser"
	"github.com/ibeckermayer/scroll4me/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "bot-test":
		runBotTest()
		os.Exit(0)
	case "open":
		if len(os.Args) < 3 {
			fmt.Println("Usage: s4m open <config|cache>")
			os.Exit(1)
		}
		runOpen(os.Args[2])
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: s4m <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  bot-test       Open bot.sannysoft.com to audit browser fingerprint")
	fmt.Println("  open config   Open config file in default editor")
	fmt.Println("  open cache    Open cache directory in file explorer")
}

func runBotTest() {
	log.Println("Opening bot.sannysoft.com with stealth browser options...")

	opts := browseropts.Options(false) // non-headless so you can see it

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	go func() {
		err := chromedp.Run(ctx,
			chromedp.Navigate("https://bot.sannysoft.com"),
		)
		if err != nil {
			log.Printf("Failed to navigate: %v", err)
		}
	}()

	fmt.Println("Press Enter to end program...")
	fmt.Scanln()

	log.Println("Done.")
}

func runOpen(target string) {
	var path string
	var err error

	switch target {
	case "config":
		path, err = config.ConfigPath()
	case "cache":
		path, err = config.CacheDir()
	default:
		fmt.Printf("Unknown target: %s\n", target)
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("Failed to get path: %v", err)
	}

	if err := browser.OpenFile(path); err != nil {
		log.Fatalf("Failed to open: %v", err)
	}
}
