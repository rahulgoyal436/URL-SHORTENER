package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"url-shortener/internal/model"
)

type Repo struct {
	DB *sql.DB
}

func NewRepo(db *sql.DB) *Repo {
	return &Repo{DB: db}
}

func (r *Repo) GetByShortCode(ctx context.Context, code string) (*model.URLMapping, error) {
	q := `SELECT id, short_code, original_url, created_at, click_count, last_accessed_at FROM url_mappings WHERE short_code = $1`
	var m model.URLMapping
	row := r.DB.QueryRowContext(ctx, q, code)
	var lastAccess sql.NullTime
	if err := row.Scan(&m.ID, &m.ShortCode, &m.OriginalURL, &m.CreatedAt, &m.ClickCount, &lastAccess); err != nil {
		return nil, err
	}
	if lastAccess.Valid {
		t := lastAccess.Time
		m.LastAccessedAt = &t
	}
	return &m, nil
}

func (r *Repo) GetByOriginalURL(ctx context.Context, original string) (*model.URLMapping, error) {
	q := `SELECT id, short_code, original_url, created_at, click_count, last_accessed_at FROM url_mappings WHERE original_url = $1`
	var m model.URLMapping
	row := r.DB.QueryRowContext(ctx, q, original)
	var lastAccess sql.NullTime
	if err := row.Scan(&m.ID, &m.ShortCode, &m.OriginalURL, &m.CreatedAt, &m.ClickCount, &lastAccess); err != nil {
		return nil, err
	}
	if lastAccess.Valid {
		t := lastAccess.Time
		m.LastAccessedAt = &t
	}
	return &m, nil
}

func (r *Repo) Create(ctx context.Context, code, original string) (*model.URLMapping, error) {
	q := `INSERT INTO url_mappings (short_code, original_url) VALUES ($1, $2) RETURNING id, created_at`
	var id int64
	var created time.Time
	err := r.DB.QueryRowContext(ctx, q, code, original).Scan(&id, &created)
	if err != nil {
		return nil, err
	}
	return &model.URLMapping{
		ID: id, ShortCode: code, OriginalURL: original, CreatedAt: created, ClickCount: 0,
	}, nil
}

func (r *Repo) IncrementClickBy(ctx context.Context, code string, delta int64) error {
	// Update click_count and last_accessed_at
	q := `
		UPDATE url_mappings
		SET click_count = click_count + $2, last_accessed_at = now()
		WHERE short_code = $1
	`
	_, err := r.DB.ExecContext(ctx, q, code, delta)
	return err
}

func (r *Repo) List(ctx context.Context, offset, limit int) ([]model.URLMapping, error) {
	q := `SELECT id, short_code, original_url, created_at, click_count, last_accessed_at FROM url_mappings ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := r.DB.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := make([]model.URLMapping, 0, limit)
	for rows.Next() {
		var m model.URLMapping
		var lastAccess sql.NullTime
		if err := rows.Scan(&m.ID, &m.ShortCode, &m.OriginalURL, &m.CreatedAt, &m.ClickCount, &lastAccess); err != nil {
			return nil, err
		}
		if lastAccess.Valid {
			t := lastAccess.Time
			m.LastAccessedAt = &t
		}
		res = append(res, m)
	}
	return res, nil
}

// ErrNotFound sentinel
var ErrNotFound = errors.New("not found")
