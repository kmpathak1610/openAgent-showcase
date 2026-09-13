package browser

import (
	"fmt"
	"os"

	"github.com/mxschmitt/playwright-go"
)

// TryPlaywright checks if Playwright is available; returns error if not.
// In production, this will attempt to launch chromium; in CI/test without browsers, returns unavailable.
func TryPlaywright() error {
	if os.Getenv("PLAYWRIGHT_ENABLED") != "true" {
		return fmt.Errorf("browser capability not enabled: set PLAYWRIGHT_ENABLED=true and install browsers via 'go run github.com/mxschmitt/playwright-go/cmd/playwright install --with-deps chromium'")
	}
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("playwright run failed: %w", err)
	}
	defer func() {
		_ = pw.Stop()
	}()
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("chromium launch failed (browsers not installed?): %w", err)
	}
	_ = browser.Close()
	return nil
}

// IsAvailable reports if browser engine is considered available
func IsAvailable() bool {
	return TryPlaywright() == nil
}
