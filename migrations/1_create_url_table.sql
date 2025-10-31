CREATE TABLE IF NOT EXISTS url_mappings (
  id SERIAL PRIMARY KEY,
  short_code VARCHAR(16) NOT NULL UNIQUE,
  original_url TEXT NOT NULL UNIQUE,
  click_count BIGINT DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
  last_accessed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_short_code ON url_mappings (short_code);
CREATE INDEX IF NOT EXISTS idx_original_url ON url_mappings (original_url);
