package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"url-shortener/internal/service"

	"github.com/gorilla/mux"
)

type Handler struct {
	Service     *service.Service
	AdminToken  string
	RateLimiter *SimpleRateLimiter
}

// Request bodies
type shortenRequest struct {
	URL         string `json:"url"`
	CustomAlias string `json:"custom_alias,omitempty"`
}

type shortenResponse struct {
	ShortURL    string `json:"short_url"`
	ShortCode   string `json:"short_code"`
	OriginalURL string `json:"original_url"`
}

func NewHandler(s *service.Service) *Handler {
	h := &Handler{
		Service:     s,
		AdminToken:  os.Getenv("ADMIN_TOKEN"),
		RateLimiter: NewSimpleRateLimiter(),
	}
	return h
}

func (h *Handler) Routes() http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/shorten", h.RateLimitMiddleware(h.CreateShort)).Methods("POST")
	r.HandleFunc("/admin/urls", h.AdminAuth(h.ListURLs)).Methods("GET")
	r.HandleFunc("/healthz", h.Healthz).Methods("GET")
	r.HandleFunc("/{code}", h.Redirect).Methods("GET")

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			log.Println("request:", req.Method, req.URL.Path)
			next.ServeHTTP(w, req)
		})
	})

	return r
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (h *Handler) CreateShort(w http.ResponseWriter, r *http.Request) {
	var req shortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url missing", http.StatusBadRequest)
		return
	}

	// Optional: allow custom alias - validate and ensure length & uniqueness
	if req.CustomAlias != "" {
		if len(req.CustomAlias) > 10 {
			http.Error(w, "custom alias too long", http.StatusBadRequest)
			return
		}
		// check if alias exists
		if _, err := h.Service.Resolve(r.Context(), req.CustomAlias); err == nil {
			http.Error(w, "alias already taken", http.StatusConflict)
			return
		}
		// store by calling Repo directly could be available through service; for simplicity, attempt to create by bypassing deterministic
		// (You might adapt Service to accept custom alias)
		// For now we return Not Implemented behavior
		http.Error(w, "custom alias not implemented in this build", http.StatusNotImplemented)
		return
	}

	m, err := h.Service.CreateShort(r.Context(), req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	host := r.Host
	scheme := "https"
	if r.TLS == nil && os.Getenv("DEV_HTTP") == "true" {
		scheme = "http"
	}
	short := fmt.Sprintf("%s://%s/%s", scheme, host, m.ShortCode)
	resp := &shortenResponse{
		ShortURL:    short,
		ShortCode:   m.ShortCode,
		OriginalURL: m.OriginalURL,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	code := vars["code"]
	if code == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Rate-limit redirect by IP as well
	ip := r.RemoteAddr
	if !h.RateLimiter.Allow(ip) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	original, err := h.Service.Resolve(r.Context(), code)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, original, http.StatusFound)
}

func (h *Handler) ListURLs(w http.ResponseWriter, r *http.Request) {
	pageQ := r.URL.Query().Get("page")
	limitQ := r.URL.Query().Get("limit")
	page := 1
	limit := 20
	if pageQ != "" {
		if p, err := strconv.Atoi(pageQ); err == nil {
			page = p
		}
	}
	if limitQ != "" {
		if l, err := strconv.Atoi(limitQ); err == nil {
			limit = l
		}
	}
	list, err := h.Service.List(r.Context(), page, limit)
	if err != nil {
		http.Error(w, "error listing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (h *Handler) AdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Admin-Token")
		if token == "" || token != h.AdminToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (h *Handler) RateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if !h.RateLimiter.Allow(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	}
}
