//go:build !linux

package connectedprocess

import "context"

func Audit(context.Context, int, string) (Identity, error) { return Identity{}, ErrUnsafe }
