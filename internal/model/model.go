package model

import "time"

type URLMapping struct {
	ID             int64      `db:"id" json:"id"`
	ShortCode      string     `db:"short_code" json:"short_code"`
	OriginalURL    string     `db:"original_url" json:"original_url"`
	ClickCount     int64      `db:"click_count" json:"click_count"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
	LastAccessedAt *time.Time `db:"last_accessed_at" json:"last_accessed_at,omitempty"`
}
