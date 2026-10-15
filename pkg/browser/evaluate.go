package browser

import (
	"context"
	"encoding/json"
	"fmt"

	"emperror.dev/errors"
	"github.com/chromedp/chromedp"
)

// Evaluate runs a JavaScript expression and decodes the result into the target destination pointer.
func (b *Browser) Evaluate(ctx context.Context, script string, target any) error {
	actCtx, cancel, err := b.actionContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	if err := chromedp.Run(actCtx, chromedp.Evaluate(script, target)); err != nil {
		return errors.Wrapf(err, "failed to evaluate javascript: %s", script)
	}
	return nil
}

// EvaluateGeneric is a type-safe generic helper function to evaluate a JavaScript expression and return type T.
func EvaluateGeneric[T any](ctx context.Context, b *Browser, script string) (T, error) {
	var result T
	if err := b.Evaluate(ctx, script, &result); err != nil {
		return result, err
	}
	return result, nil
}

// EvaluateFunction evaluates a JavaScript function by name, automatically serializing arguments to JSON.
func EvaluateFunction[T any](ctx context.Context, b *Browser, fnName string, args ...any) (T, error) {
	var zero T
	argsJSON := ""
	for i, arg := range args {
		argBytes, err := json.Marshal(arg)
		if err != nil {
			return zero, errors.Wrapf(err, "failed to marshal argument index %d for function '%s'", i, fnName)
		}
		if i > 0 {
			argsJSON += ", "
		}
		argsJSON += string(argBytes)
	}

	script := fmt.Sprintf("%s(%s)", fnName, argsJSON)
	return EvaluateGeneric[T](ctx, b, script)
}
