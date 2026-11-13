//go:build !linux

package actionworker

import "context"

func RunExchange(context.Context) error { return ErrUnsafe }
