# URL-SHORTENER

Instructions to run the project

Navigate to the root of the repositiory

docker-compose up --build

# URL-SHORTENER Project File Overview

## Project Structure & File Explanations

### Root Directory Files

#### `Readme.md`

- **Purpose**: Basic project documentation
- **Content**: Simple instructions to run the project using Docker Compose
- **Commands**: `docker-compose up --build`

#### `go.mod`

- **Purpose**: Go module definition and dependency management
- **Content**: Defines module name `url-shortener` with Go 1.24.0
- **Dependencies**: Gorilla handlers/mux, PostgreSQL driver (pgx), Redis client, godotenv

#### `go.sum`

- **Purpose**: Dependency checksums for security and reproducibility
- **Content**: Cryptographic hashes of all direct and indirect dependencies

#### `docker-compose.yml`

- **Purpose**: Multi-container Docker application orchestration
- **Services**:
  - **db**: PostgreSQL 15 database with health checks
  - **redis**: Redis 7 for caching
  - **app**: Go application with environment variables
- **Features**: Service dependencies, health checks, port mapping

#### `Dockerfile`

- **Purpose**: Multi-stage Docker build for Go application
- **Stages**:
  - **Builder**: Compiles Go binary with CGO disabled
  - **Final**: Minimal Alpine image with CA certificates
- **Output**: Optimized production-ready container

### Application Entry Point

#### `cmd/server/main.go`

- **Purpose**: Application entry point and server initialization
- **Functions**:
  - `main()`:
    - `godotenv.Load()`: Loads environment variables from .env file, logs if not found
    - `os.Getenv("DATABASE_DSN")`: Gets database connection string, exits if missing
    - `sql.Open("pgx", dsn)`: Opens PostgreSQL connection using pgx driver
    - `db.Ping()`: Tests database connectivity, exits on failure
    - `redis.NewClient()`: Creates Redis client if REDIS_ADDR is set
    - `rdb.Ping()`: Tests Redis connectivity, continues without Redis if fails
    - `repository.NewRepo(db)`: Creates repository layer with database
    - `service.NewService(repo, rdb)`: Creates service layer with repo and Redis
    - `handler.NewHandler(svc)`: Creates HTTP handler with service
    - `h.Routes()`: Sets up HTTP routes and middleware
    - `handlers.CORS()`: Configures CORS with allowed origins, headers, methods
    - `http.Server{}`: Creates server with timeouts (5s read, 10s write, 120s idle)
    - `srv.ListenAndServe()`: Starts server in goroutine
    - `signal.Notify()`: Sets up graceful shutdown on SIGINT/SIGTERM
    - `srv.Shutdown()`: Gracefully shuts down server with 10s timeout
    - `rdb.Close()` & `db.Close()`: Closes connections on shutdown
- **Architecture**: Follows clean architecture pattern with repository-service-handler layers

### Internal Package Structure

#### `internal/model/model.go`

- **Purpose**: Data models and structures
- **Content**: `URLMapping` struct with database tags
- **Fields**: ID, ShortCode, OriginalURL, ClickCount, timestamps
- **Features**: JSON serialization support

#### `internal/repository/repository.go`

- **Purpose**: Database access layer
- **Functions**:
  - `NewRepo(db *sql.DB)`:
    - `return &Repo{DB: db}`: Creates repository struct with database connection
  - `GetByShortCode(ctx, code)`:
    - `SELECT id, short_code...`: Queries url_mappings table by short_code
    - `r.DB.QueryRowContext(ctx, q, code)`: Executes parameterized query with context
    - `row.Scan(&m.ID, &m.ShortCode...)`: Scans result into URLMapping struct
    - `sql.NullTime`: Handles nullable last_accessed_at timestamp
    - `if lastAccess.Valid`: Converts nullable time to pointer if not null
    - `return &m, nil`: Returns populated URLMapping or error
  - `GetByOriginalURL(ctx, original)`:
    - `SELECT...WHERE original_url = $1`: Finds existing mapping by original URL
    - Same scanning logic as GetByShortCode for consistency
  - `Create(ctx, code, original)`:
    - `INSERT INTO url_mappings...RETURNING id, created_at`: Inserts new record
    - `r.DB.QueryRowContext().Scan(&id, &created)`: Gets auto-generated ID and timestamp
    - `return &model.URLMapping{...}`: Returns new URLMapping with populated fields
  - `IncrementClickBy(ctx, code, delta)`:
    - `UPDATE url_mappings SET click_count = click_count + $2`: Atomically increments counter
    - `last_accessed_at = now()`: Updates access timestamp
    - `WHERE short_code = $1`: Targets specific URL mapping
    - `r.DB.ExecContext()`: Executes update without returning rows
  - `List(ctx, offset, limit)`:
    - `SELECT...ORDER BY created_at DESC LIMIT $1 OFFSET $2`: Paginated query
    - `r.DB.QueryContext()`: Returns multiple rows
    - `defer rows.Close()`: Ensures proper resource cleanup
    - `for rows.Next()`: Iterates through result set
    - `rows.Scan()`: Scans each row into URLMapping struct
    - `res = append(res, m)`: Builds result slice
- **Features**: Context support, error handling, SQL injection prevention

#### `internal/service/service.go`

- **Purpose**: Business logic layer
- **Functions**:
  - `NewService(r, rc)`:
    - `return &Service{Repo: r, Redis: rc, ShortLen: 8}`: Creates service with 8-char short codes
  - `CreateShort(ctx, original)`:
    - `if !util.ValidateURL(original)`: Validates URL format first
    - `return nil, errors.New("invalid url")`: Rejects invalid URLs
    - `s.Repo.GetByOriginalURL(ctx, original)`: Checks for existing mapping (idempotent)
    - `if err == nil { return existing, nil }`: Returns existing if found
    - `for attempt := 0; attempt < 16; attempt++`: Tries 16 collision attempts
    - `code := util.DeterministicShortCode(original, s.ShortLen, attempt)`: Generates code
    - `s.Repo.GetByShortCode(ctx, code)`: Checks if code exists
    - `if m.OriginalURL == original`: Same URL, return existing mapping
    - `continue`: Different URL collision, try next attempt
    - `created, err := s.Repo.Create(ctx, code, original)`: Creates new mapping
    - `if err != nil { continue }`: Race condition, try next attempt
    - `s.Redis.Set(ctx, "short:"+code, original, 0)`: Cache in Redis indefinitely
    - `return created, nil`: Success, return new mapping
  - `Resolve(ctx, code)`:
    - `s.Redis.Get(ctx, "short:"+code).Result()`: Try Redis cache first
    - `if err == nil`: Cache hit, use cached value
    - `s.Redis.Incr(ctx, "clicks:"+code)`: Increment Redis counter
    - `go s.persistClick(context.Background(), code)`: Async DB update
    - `return val, nil`: Return cached URL
    - `m, err := s.Repo.GetByShortCode(ctx, code)`: Cache miss, query DB
    - `s.Redis.Set(ctx, "short:"+code, m.OriginalURL, 24*time.Hour)`: Cache for 24h
    - `go s.persistClick()`: Async click tracking
    - `return m.OriginalURL, nil`: Return DB result
  - `persistClick(ctx, code)`:
    - `cnt, err := s.Redis.Get(ctx, "clicks:"+code).Int64()`: Get Redis counter
    - `if err == nil && cnt > 0`: Valid counter exists
    - `s.Repo.IncrementClickBy(ctx, code, cnt)`: Bulk update DB
    - `s.Redis.Del(ctx, "clicks:"+code)`: Clear Redis counter
    - `s.Repo.IncrementClickBy(ctx, code, 1)`: Fallback: increment by 1
  - `List(ctx, page, limit)`:
    - `if page < 1 { page = 1 }`: Validate page number
    - `if limit < 1 || limit > 100 { limit = 20 }`: Validate and cap limit
    - `offset := (page - 1) * limit`: Calculate SQL offset
    - `return s.Repo.List(ctx, offset, limit)`: Delegate to repository
- **Caching Strategy**: Redis for fast lookups, async DB persistence

#### `internal/handler/handler.go`

- **Purpose**: HTTP request handling and routing
- **Functions**:
  - `NewHandler(s)`:
    - `h := &Handler{Service: s, AdminToken: os.Getenv("ADMIN_TOKEN"), RateLimiter: NewSimpleRateLimiter()}`: Creates handler struct
    - `return h`: Returns configured handler
  - `Routes()`:
    - `r := mux.NewRouter()`: Creates Gorilla mux router
    - `r.HandleFunc("/shorten", h.RateLimitMiddleware(h.CreateShort)).Methods("POST")`: Shorten endpoint with rate limiting
    - `r.HandleFunc("/admin/urls", h.AdminAuth(h.ListURLs)).Methods("GET")`: Admin-only list endpoint
    - `r.HandleFunc("/healthz", h.Healthz).Methods("GET")`: Health check endpoint
    - `r.HandleFunc("/{code}", h.Redirect).Methods("GET")`: Redirect endpoint
    - `r.Use(func(next http.Handler)...)`: Adds logging middleware to all routes
    - `log.Println("request:", req.Method, req.URL.Path)`: Logs all requests
    - `return r`: Returns configured router
  - `Healthz(w, r)`:
    - `w.WriteHeader(http.StatusOK)`: Sets 200 status
    - `w.Write([]byte("ok"))`: Returns "ok" response
  - `CreateShort(w, r)`:
    - `var req shortenRequest`: Declares request struct
    - `json.NewDecoder(r.Body).Decode(&req)`: Parses JSON body
    - `if err != nil { http.Error(w, "invalid body", 400) }`: Validates JSON
    - `if req.URL == "" { http.Error(w, "url missing", 400) }`: Validates URL field
    - `if req.CustomAlias != ""`: Handles custom alias (not implemented)
    - `if len(req.CustomAlias) > 10`: Validates alias length
    - `s.Service.Resolve(r.Context(), req.CustomAlias)`: Checks alias availability
    - `http.Error(w, "alias already taken", 409)`: Returns conflict error
    - `m, err := h.Service.CreateShort(r.Context(), req.URL)`: Creates short URL
    - `host := r.Host`: Gets request host
    - `scheme := "https"`: Defaults to HTTPS
    - `if r.TLS == nil && os.Getenv("DEV_HTTP") == "true"`: Uses HTTP in dev mode
    - `short := fmt.Sprintf("%s://%s/%s", scheme, host, m.ShortCode)`: Builds full URL
    - `resp := &shortenResponse{...}`: Creates response struct
    - `w.Header().Set("Content-Type", "application/json")`: Sets JSON header
    - `json.NewEncoder(w).Encode(resp)`: Returns JSON response
  - `Redirect(w, r)`:
    - `vars := mux.Vars(r)`: Gets URL variables
    - `code := vars["code"]`: Extracts short code
    - `if code == "" { http.Error(w, "not found", 404) }`: Validates code
    - `ip := r.RemoteAddr`: Gets client IP
    - `if !h.RateLimiter.Allow(ip)`: Checks rate limit
    - `http.Error(w, "rate limit exceeded", 429)`: Returns rate limit error
    - `original, err := h.Service.Resolve(r.Context(), code)`: Resolves short code
    - `if err != nil { http.Error(w, "not found", 404) }`: Returns 404 if not found
    - `http.Redirect(w, r, original, http.StatusFound)`: Redirects to original URL
  - `ListURLs(w, r)`:
    - `pageQ := r.URL.Query().Get("page")`: Gets page query parameter
    - `limitQ := r.URL.Query().Get("limit")`: Gets limit query parameter
    - `page := 1; limit := 20`: Sets defaults
    - `if pageQ != "" { if p, err := strconv.Atoi(pageQ)... }`: Parses page number
    - `if limitQ != "" { if l, err := strconv.Atoi(limitQ)... }`: Parses limit
    - `list, err := h.Service.List(r.Context(), page, limit)`: Gets paginated list
    - `json.NewEncoder(w).Encode(list)`: Returns JSON response
  - `AdminAuth(next)`:
    - `return func(w http.ResponseWriter, r *http.Request)`: Returns middleware function
    - `token := r.Header.Get("X-Admin-Token")`: Gets admin token from header
    - `if token == "" || token != h.AdminToken`: Validates token
    - `http.Error(w, "unauthorized", 401)`: Returns unauthorized error
    - `next.ServeHTTP(w, r)`: Calls next handler if authorized
  - `RateLimitMiddleware(next)`:
    - `return func(w http.ResponseWriter, r *http.Request)`: Returns middleware function
    - `ip := r.RemoteAddr`: Gets client IP
    - `if !h.RateLimiter.Allow(ip)`: Checks rate limit
    - `http.Error(w, "rate limit exceeded", 429)`: Returns rate limit error
    - `next.ServeHTTP(w, r)`: Calls next handler if allowed
- **Endpoints**: POST /shorten, GET /{code}, GET /admin/urls, GET /healthz

#### `internal/handler/rate_limiter.go`

- **Purpose**: In-memory rate limiting implementation
- **Functions**:
  - `NewSimpleRateLimiter()`:
    - `return &SimpleRateLimiter{buckets: make(map[string]*tokenBucket), rate: 1.0, burst: 10}`: Creates limiter with token buckets map, 1 token/sec rate, 10 token burst
  - `Allow(key)`:
    - `s.mu.Lock(); defer s.mu.Unlock()`: Thread-safe access to buckets map
    - `b, ok := s.buckets[key]`: Gets existing bucket for key (IP)
    - `now := time.Now()`: Gets current time
    - `if !ok`: First request from this IP
    - `s.buckets[key] = &tokenBucket{tokens: s.burst - 1, last: now}`: Creates new bucket with burst-1 tokens
    - `return true`: Allows first request
    - `elapsed := now.Sub(b.last).Seconds()`: Calculates time since last request
    - `b.tokens += elapsed * s.rate`: Adds tokens based on elapsed time and rate
    - `if b.tokens > s.burst { b.tokens = s.burst }`: Caps tokens at burst limit
    - `b.last = now`: Updates last access time
    - `if b.tokens >= 1`: Has at least 1 token
    - `b.tokens -= 1`: Consumes 1 token
    - `return true`: Allows request
    - `return false`: No tokens available, denies request
- **Algorithm**: Token bucket per IP address with time-based replenishment
- **Note**: In-memory only - not suitable for multi-instance deployment

#### `internal/util/util.go`

- **Purpose**: Utility functions
- **Functions**:
  - `ValidateURL(raw)`:
    - `u, err := url.ParseRequestURI(strings.TrimSpace(raw))`: Parses URL after trimming whitespace
    - `if err != nil { return false }`: Invalid URL format
    - `if u.Scheme != "http" && u.Scheme != "https" { return false }`: Only allows HTTP/HTTPS
    - `return true`: Valid URL
  - `base62Encode(num)`:
    - `if num == 0 { return "0" }`: Handle zero case
    - `b := make([]byte, 0)`: Create byte slice for result
    - `for num > 0`: Convert number to base62
    - `rem := num % 62`: Get remainder (0-61)
    - `b = append([]byte{base62Chars[rem]}, b...)`: Prepend character to result
    - `num /= 62`: Divide by base
    - `return string(b)`: Convert bytes to string
  - `DeterministicShortCode(original, length, attempt)`:
    - `h := sha256.Sum256([]byte(original))`: Hash original URL with SHA256
    - `offset := attempt % (len(h) - 8 + 1)`: Calculate offset based on attempt (0-24)
    - `slice := h[offset : offset+8]`: Extract 8 bytes from hash
    - `num := binary.BigEndian.Uint64(slice)`: Convert bytes to uint64
    - `if length >= 10`: Use full number for long codes
    - `else`: Reduce number for shorter codes
    - `limit := uint64(math.Pow(62, float64(length)))`: Calculate max value for length
    - `num = num % limit`: Reduce number to fit length
    - `code := base62Encode(num)`: Encode to base62
    - `if len(code) > length { code = code[:length] }`: Truncate if too long
    - `else if len(code) < length`: Pad if too short
    - `code = strings.Repeat("0", length-len(code)) + code`: Left-pad with zeros
    - `return code`: Return deterministic short code
- **Features**: SHA256-based deterministic generation with collision handling

### Database

#### `migrations/1_create_url_table.sql`

- **Purpose**: Database schema initialization
- **Table**: `url_mappings` with indexes
- **Fields**: Auto-increment ID, unique constraints, timestamps
- **Indexes**: Optimized for short_code and original_url lookups
- **Features**: Click tracking, last access timestamp

## Architecture Overview

This is a **clean architecture** Go application with:

1. **Layered Design**: Handler → Service → Repository → Database
2. **Dependency Injection**: Dependencies passed through constructors
3. **Caching Strategy**: Redis for performance, PostgreSQL for persistence
4. **Rate Limiting**: Token bucket algorithm per IP
5. **Containerization**: Docker with multi-stage builds
6. **Graceful Shutdown**: Proper resource cleanup
7. **Admin Features**: Protected endpoints with token authentication

## Key Features

- **Deterministic Short Codes**: Same URL always gets same short code
- **Collision Handling**: Multiple attempts with different hash offsets
- **Performance Optimization**: Redis caching with async DB updates
- **Security**: Rate limiting, URL validation, admin authentication
- **Scalability**: Stateless design (except in-memory rate limiter)
- **Monitoring**: Health checks and click tracking

