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

	tasks := chromedp.Tasks{
		chromedp.Navigate(targetURL),
	}
	if waitSelector != "" {
		tasks = append(tasks, chromedp.WaitReady(waitSelector, chromedp.ByQuery))
	}

	if err := chromedp.Run(actCtx, tasks); err != nil {
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

	var currentURL string
	if err := chromedp.Run(actCtx, chromedp.Location(&currentURL)); err != nil {
		return "", errors.Wrap(err, "failed to get current browser URL")
	}

	return currentURL, nil
}
