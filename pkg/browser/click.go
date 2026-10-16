package browser

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"emperror.dev/errors"
	"github.com/chromedp/chromedp"
)

// ClickOptions specifies target selection, coordinates, and behavior for mouse click operations.
type ClickOptions struct {
	// Selector is an optional CSS query selector to locate and click an element.
	Selector string `json:"selector"`
	// WaitVisible instructs the engine to wait until the element is visible before clicking.
	WaitVisible bool `json:"wait_visible"`
	// X specifies the horizontal coordinate in pixels when clicking by coordinates (used when Selector is empty).
	X float64 `json:"x"`
	// Y specifies the vertical coordinate in pixels when clicking by coordinates (used when Selector is empty).
	Y float64 `json:"y"`
	// Button specifies the mouse button: "left" (default), "middle", or "right".
	Button string `json:"button"`
	// ClickCount specifies the number of clicks (default is 1; 2 for double-click).
	ClickCount int `json:"click_count"`
}

// Click triggers a mouse click on an element located by selector or at specified (X, Y) coordinates.
func (b *Browser) Click(ctx context.Context, opts ClickOptions) error {
	actCtx, cancel, err := b.actionContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	btnCode := 0 // 0 = left, 1 = middle, 2 = right
	switch opts.Button {
	case "middle":
		btnCode = 1
	case "right":
		btnCode = 2
	}

	if opts.Selector != "" {
		// Wait for visibility if requested
		if opts.WaitVisible {
			waitScript := fmt.Sprintf(`(() => {
				const el = document.querySelector(%s);
				if (!el) return false;
				const r = el.getBoundingClientRect();
				const style = window.getComputedStyle(el);
				return r.width > 0 && r.height > 0 && style.visibility !== 'hidden' && style.display !== 'none';
			})()`, strconv.Quote(opts.Selector))

			ticker := time.NewTicker(25 * time.Millisecond)
			defer ticker.Stop()

			for {
				isVisible, err := chromedp.Run(actCtx, chromedp.Evaluate[bool](waitScript))
				if err != nil {
					return errors.Wrapf(err, "failed evaluating visibility for selector '%s'", opts.Selector)
				}
				if isVisible {
					break
				}
				select {
				case <-actCtx.Done():
					return errors.Wrapf(actCtx.Err(), "timeout waiting for selector '%s' to become visible", opts.Selector)
				case <-ticker.C:
				}
			}
		}

		clickScript := fmt.Sprintf(`(() => {
			const el = document.querySelector(%s);
			if (!el) return false;
			const r = el.getBoundingClientRect();
			const x = r.left + r.width / 2;
			const y = r.top + r.height / 2;
			const eventOpts = { bubbles: true, cancelable: true, view: window, clientX: x, clientY: y, button: %d };
			el.dispatchEvent(new MouseEvent('mousedown', eventOpts));
			el.dispatchEvent(new MouseEvent('mouseup', eventOpts));
			el.dispatchEvent(new MouseEvent('click', eventOpts));
			if (typeof el.click === 'function') {
				el.click();
			}
			return true;
		})()`, strconv.Quote(opts.Selector), btnCode)

		success, err := chromedp.Run(actCtx, chromedp.Evaluate[bool](clickScript))
		if err != nil {
			return errors.Wrapf(err, "failed to click selector '%s'", opts.Selector)
		}
		if !success {
			return errors.Errorf("element for selector '%s' not found in DOM", opts.Selector)
		}
		return nil
	}

	// Coordinate-based click
	coordScript := fmt.Sprintf(`(() => {
		const x = %f;
		const y = %f;
		const el = document.elementFromPoint(x, y) || document.body;
		if (!el) return false;
		const eventOpts = { bubbles: true, cancelable: true, view: window, clientX: x, clientY: y, button: %d };
		el.dispatchEvent(new MouseEvent('mousedown', eventOpts));
		el.dispatchEvent(new MouseEvent('mouseup', eventOpts));
		el.dispatchEvent(new MouseEvent('click', eventOpts));
		if (typeof el.click === 'function') {
			el.click();
		}
		return true;
	})()`, opts.X, opts.Y, btnCode)

	success, err := chromedp.Run(actCtx, chromedp.Evaluate[bool](coordScript))
	if err != nil {
		return errors.Wrapf(err, "failed to click at coordinates (%.1f, %.1f)", opts.X, opts.Y)
	}
	if !success {
		return errors.Errorf("no element found at coordinates (%.1f, %.1f)", opts.X, opts.Y)
	}

	return nil
}
