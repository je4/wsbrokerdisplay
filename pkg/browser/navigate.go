package browser

import (
	"context"

	"emperror.dev/errors"
	"github.com/chromedp/chromedp"
)

// Navigate loads the specified URL into the active browser page and waits for the body DOM ready state.
func (b *Browser) Navigate(ctx context.Context, targetURL string) error {
	return b.NavigateWait(ctx, targetURL, "body")
}

// NavigateWait loads the specified URL into the active browser page and waits until waitSelector is ready.
func (b *Browser) NavigateWait(ctx context.Context, targetURL string, waitSelector string) error {
	actCtx, cancel, err := b.actionContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	steps := []chromedp.Action[chromedp.Void]{
		chromedp.Navigate(targetURL),
	}
	if waitSelector != "" {
		steps = append(steps, chromedp.WaitReady(waitSelector))
	}

	if err := chromedp.Do(actCtx, steps...); err != nil {
		return errors.Wrapf(err, "failed to navigate to URL '%s'", targetURL)
	}

	return nil
}

// GetURL retrieves the current primary page URL.
func (b *Browser) GetURL(ctx context.Context) (string, error) {
	actCtx, cancel, err := b.actionContext(ctx)
	if err != nil {
		return "", err
	}
	defer cancel()

	currentURL, err := chromedp.Run(actCtx, chromedp.Location())
	if err != nil {
		return "", errors.Wrap(err, "failed to get current browser URL")
	}

	return currentURL, nil
}
