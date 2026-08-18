package reputation

import (
	"context"
	"time"
)

type Result struct {
	Unsafe      bool      `json:"unsafe"`
	ThreatTypes []string  `json:"threatTypes,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt,omitempty"`
}

type Checker interface {
	Check(context.Context, string) (Result, error)
}

type AllowAll struct{}

func (AllowAll) Check(context.Context, string) (Result, error) {
	return Result{}, nil
}
