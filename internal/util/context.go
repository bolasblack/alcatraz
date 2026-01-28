package util

import (
	"context"

	"github.com/bolasblack/alcatraz/internal/transact"
)

// fsKey is the context key for TransactFs.
type fsKey struct{}

// WithFs returns a new context with the given TransactFs.
func WithFs(ctx context.Context, fs *transact.TransactFs) context.Context {
	return context.WithValue(ctx, fsKey{}, fs)
}

// GetFs returns the TransactFs from context, or nil if not set.
func GetFs(ctx context.Context) *transact.TransactFs {
	if fs, ok := ctx.Value(fsKey{}).(*transact.TransactFs); ok {
		return fs
	}
	return nil
}

// MustGetFs returns the TransactFs from context, panicking if not set.
// Use this in *WithContext functions where TransactFs is required.
// TransactFs implements afero.Fs, so it can be used directly for file operations.
func MustGetFs(ctx context.Context) *transact.TransactFs {
	fs := GetFs(ctx)
	if fs == nil {
		panic("util: TransactFs not found in context - use WithFs to set it")
	}
	return fs
}
