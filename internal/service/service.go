package service

import (
	"context"
	"errors"
	"time"
	"url-shortener/internal/model"
	"url-shortener/internal/repository"
	"url-shortener/internal/util"

	"github.com/redis/go-redis/v9"
)

type Service struct {
	Repo     *repository.Repo
	Redis    *redis.Client // may be nil if disabled
	ShortLen int
}

func NewService(r *repository.Repo, rc *redis.Client) *Service {
	return &Service{
		Repo: r, Redis: rc, ShortLen: 8,
	}
}

func (s *Service) CreateShort(ctx context.Context, original string) (*model.URLMapping, error) {
	// validate
	if !util.ValidateURL(original) {
		return nil, errors.New("invalid url")
	}

	// check if original already exists (idempotent)
	if existing, err := s.Repo.GetByOriginalURL(ctx, original); err == nil {
		return existing, nil
	}

	// generate deterministic codes in attempts
	for attempt := 0; attempt < 16; attempt++ {
		code := util.DeterministicShortCode(original, s.ShortLen, attempt)
		// check if code exists
		if m, err := s.Repo.GetByShortCode(ctx, code); err == nil {
			if m.OriginalURL == original {
				// same mapping; return
				return m, nil
			}
			// collision with different URL -> continue attempts
			continue
		} else {
			// no mapping with that code; attempt to create
			created, err := s.Repo.Create(ctx, code, original)
			if err != nil {
				// might be race condition; try next
				continue
			}
			// cache in redis
			if s.Redis != nil {
				_ = s.Redis.Set(ctx, "short:"+code, original, 0).Err()
			}
			return created, nil
		}
	}

	// last resort: append timestamp-based suffix (non-deterministic) - unlikely
	return nil, errors.New("failed to generate a unique short code")
}

func (s *Service) Resolve(ctx context.Context, code string) (string, error) {
	// try cache first
	if s.Redis != nil {
		if val, err := s.Redis.Get(ctx, "short:"+code).Result(); err == nil {
			// increment click counter in redis (fast)
			_ = s.Redis.Incr(ctx, "clicks:"+code).Err()
			// schedule persistent update asynchronously
			go s.persistClick(context.Background(), code)
			return val, nil
		}
	}

	// db lookup
	m, err := s.Repo.GetByShortCode(ctx, code)
	if err != nil {
		return "", err
	}
	// populate cache
	if s.Redis != nil {
		_ = s.Redis.Set(ctx, "short:"+code, m.OriginalURL, 24*time.Hour).Err()
		_ = s.Redis.Incr(ctx, "clicks:"+code).Err()
		go s.persistClick(context.Background(), code)
	} else {
		// no redis - update DB asynchronously
		go s.persistClick(context.Background(), code)
	}
	return m.OriginalURL, nil
}

func (s *Service) persistClick(ctx context.Context, code string) {
	if s.Redis != nil {
		cnt, err := s.Redis.Get(ctx, "clicks:"+code).Int64()
		if err == nil && cnt > 0 {
			// One bulk DB update
			_ = s.Repo.IncrementClickBy(ctx, code, cnt)
			// Clear counter
			_ = s.Redis.Del(ctx, "clicks:"+code).Err()
			return
		}
	}

	// fallback: just +1
	_ = s.Repo.IncrementClickBy(ctx, code, 1)
}

func (s *Service) List(ctx context.Context, page, limit int) ([]model.URLMapping, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit
	return s.Repo.List(ctx, offset, limit)
}
