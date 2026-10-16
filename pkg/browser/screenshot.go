package browser

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"image/png"

	"emperror.dev/errors"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/disintegration/imaging"
)

// ScreenshotOptions configures screenshot capture format, dimensions, and image processing.
type ScreenshotOptions struct {
	// Format specifies the image encoding ("png" or "jpeg"). Defaults to "png" if empty and Quality is 0.
	Format string `json:"format"`
	// Quality defines the compression quality for JPEG output (1-100).
	Quality int `json:"quality"`
	// FullPage determines whether to capture the full scrollable document height or only current viewport.
	FullPage bool `json:"full_page"`
	// Width defines an optional target width for image downscaling/resizing.
	Width int `json:"width"`
	// Height defines an optional target height for image downscaling/resizing.
	Height int `json:"height"`
	// MaxWidth defines an optional maximum bounding width for aspect-ratio preserving fit.
	MaxWidth int `json:"max_width"`
	// MaxHeight defines an optional maximum bounding height for aspect-ratio preserving fit.
	MaxHeight int `json:"max_height"`
	// SharpenSigma defines the Gaussian sharpening sigma factor applied after resizing (e.g. 0.5 - 1.5).
	SharpenSigma float64 `json:"sharpen_sigma"`
}

// Screenshot captures an image of the current page based on the given ScreenshotOptions.
// Returns the image binary data, the MIME content type ("image/png" or "image/jpeg"), and any error.
func (b *Browser) Screenshot(ctx context.Context, opts ScreenshotOptions) ([]byte, string, error) {
	actCtx, cancel, err := b.actionContext(ctx)
	if err != nil {
		return nil, "", err
	}
	defer cancel()

	var rawBuf []byte
	isJPEG := opts.Format == "jpeg" || (opts.Format == "" && opts.Quality > 0)

	if opts.FullPage {
		quality := opts.Quality
		if quality <= 0 || quality > 100 {
			if isJPEG {
				quality = 90
			} else {
				quality = 100
			}
		}
		var err error
		rawBuf, err = chromedp.Run(actCtx, chromedp.FullScreenshot(quality))
		if err != nil {
			return nil, "", errors.Wrap(err, "failed to capture full page screenshot")
		}
	} else if isJPEG {
		quality := opts.Quality
		if quality <= 0 || quality > 100 {
			quality = 90
		}
		fromSurface := true
		q := int64(quality)
		action := func(c context.Context, t *chromedp.Target) ([]byte, error) {
			res, err := cdp.Call(c, t, page.CaptureScreenshot, page.CaptureScreenshotParams{
				Format:      page.CaptureScreenshotFormatJpeg,
				Quality:     &q,
				FromSurface: &fromSurface,
			})
			if err != nil {
				return nil, err
			}
			return res.Data, nil
		}
		var err error
		rawBuf, err = chromedp.Run(actCtx, action)
		if err != nil {
			return nil, "", errors.Wrap(err, "failed to capture viewport screenshot")
		}
	} else {
		var err error
		rawBuf, err = chromedp.Run(actCtx, chromedp.CaptureScreenshot())
		if err != nil {
			return nil, "", errors.Wrap(err, "failed to capture viewport screenshot")
		}
	}

	// If no resizing or sharpening is needed and format matches
	if opts.Width <= 0 && opts.Height <= 0 && opts.MaxWidth <= 0 && opts.MaxHeight <= 0 && opts.SharpenSigma <= 0 {
		if isJPEG {
			return rawBuf, "image/jpeg", nil
		}
		return rawBuf, "image/png", nil
	}

	// Perform image processing (resizing / sharpening)
	img, _, err := image.Decode(bytes.NewReader(rawBuf))
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to decode raw screenshot image")
	}

	if opts.MaxWidth > 0 && opts.MaxHeight > 0 {
		img = imaging.Fit(img, opts.MaxWidth, opts.MaxHeight, imaging.Lanczos)
	} else if opts.MaxWidth > 0 {
		if img.Bounds().Dx() > opts.MaxWidth {
			img = imaging.Resize(img, opts.MaxWidth, 0, imaging.Lanczos)
		}
	} else if opts.MaxHeight > 0 {
		if img.Bounds().Dy() > opts.MaxHeight {
			img = imaging.Resize(img, 0, opts.MaxHeight, imaging.Lanczos)
		}
	} else if opts.Width > 0 || opts.Height > 0 {
		img = imaging.Resize(img, opts.Width, opts.Height, imaging.Lanczos)
	}
	if opts.SharpenSigma > 0 {
		img = imaging.Sharpen(img, opts.SharpenSigma)
	}

	outBuf := new(bytes.Buffer)
	if isJPEG {
		quality := opts.Quality
		if quality <= 0 || quality > 100 {
			quality = 90
		}
		if err := jpeg.Encode(outBuf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, "", errors.Wrap(err, "failed to encode jpeg image")
		}
		return outBuf.Bytes(), "image/jpeg", nil
	}

	if err := png.Encode(outBuf, img); err != nil {
		return nil, "", errors.Wrap(err, "failed to encode png image")
	}
	return outBuf.Bytes(), "image/png", nil
}
