package core

import (
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type Link struct {
	Slug      string    `json:"slug"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}

type Visit struct {
	Slug      string    `json:"-"`
	VisitedAt time.Time `json:"visitedAt"`
	Referer   string    `json:"referer,omitempty"`
	UserAgent string    `json:"userAgent,omitempty"`
}

type Stats struct {
	TotalVisits int64   `json:"totalVisits"`
	Recent      []Visit `json:"recentVisits"`
}
