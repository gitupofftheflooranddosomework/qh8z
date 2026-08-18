package storage

import (
	"context"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

type Store interface {
	CreateLink(context.Context, core.Link) error
	GetLink(context.Context, string) (core.Link, error)
	RecordVisit(context.Context, core.Visit) (int64, error)
	Stats(context.Context, string, int) (core.Stats, error)
	Ping(context.Context) error
	Close() error
}
