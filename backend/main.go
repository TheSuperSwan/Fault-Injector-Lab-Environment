package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type Entry struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type App struct {
	DB    *sql.DB
	Redis *redis.Client
	Ctx   context.Context
	
	// Fault injection (admin API)
	FaultsEnabled bool
	FaultsToken   string
	faultMu       sync.RWMutex
	faults        FaultState
}

// ----------------------
// Fault injection state
// ----------------------

type FaultState struct {
	Latency  LatencyFault
	HTTP500  HTTP500Fault
	DBDown   TimedToggle
	DBLock   TimedToggle
	Hang	 TimedToggle
}

type LatencyFault struct {
	Enabled bool
	DelayMs int      // how many ms to sleep
	Percent int      // 0-100
	Paths   []string // exact matches or "*"
	Until   time.Time
}

type HTTP500Fault struct {
	Enabled bool
	Percent int
	Paths   []string // exact matches or "*"
	Until   time.Time
}

type TimedToggle struct {
	Enabled bool
	Until   time.Time
}

func (t TimedToggle) active(now time.Time) bool {
	return t.Enabled && now.Before(t.Until)
}

func (f LatencyFault) active(now time.Time) bool {
	return f.Enabled && now.Before(f.Until) && f.DelayMs > 0 && f.Percent > 0
}

func (f HTTP500Fault) active(now time.Time) bool {
	return f.Enabled && now.Before(f.Until) && f.Percent > 0
}

func pathMatches(paths []string, reqPath string) bool {
	if len(paths) == 0 {
		return true
	}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "*" || p == reqPath {
			return true
		}
	}
	return false
}

func rollPercent(percent int) bool {
	if percent >= 100 {
		return true
	}
	if percent <= 0 {
		return false
	}
	// rand.Intn(100) returns 0..99
	return rand.Intn(100) < percent
}

func main() {
	app := &App{Ctx: context.Background()}

	// Seed random for percent rolls
	rand.Seed(time.Now().UnixNano())

	// Fault injection config (disabled by default)
	app.FaultsEnabled = strings.ToLower(getEnv("FAULTS_ENABLED", "false")) == "true"
	app.FaultsToken = getEnv("FAULTS_TOKEN", "")
	
	// Initiera databas
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "guestbook")
	dbPass := getEnv("DB_PASSWORD", "password")
	dbName := getEnv("DB_NAME", "guestbook")
	
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPass, dbName)
	
	var err error
	app.DB, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("Kunde inte ansluta till databasen:", err)
	}
	defer app.DB.Close()
	
	// Vänta på databas
	for i := 0; i < 30; i++ {
		err = app.DB.Ping()
		if err == nil {
			break
		}
		log.Println("Väntar på databas...")
		time.Sleep(2 * time.Second)
	}
	
	if err != nil {
		log.Fatal("Databas inte tillgänglig:", err)
	}
	
	log.Println("✓ Ansluten till PostgreSQL")
	
	// Skapa tabell
	app.initDB()
	
	// Initiera Redis
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	redisPass := getEnv("REDIS_PASSWORD", "")
	
	app.Redis = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password: redisPass,
		DB:       0,
	})
	
	_, err = app.Redis.Ping(app.Ctx).Result()
	if err != nil {
		log.Println("⚠ Redis inte tillgänglig, fortsätter utan cache:", err)
	} else {
		log.Println("✓ Ansluten till Redis")
	}
	
	// Setup router
	r := mux.NewRouter()
	
	// CORS middleware
	r.Use(corsMiddleware)

	// Fault middleware (only does something if FAULTS_ENABLED=true)
	r.Use(app.faultMiddleware)

	// Routes
	r.HandleFunc("/health", app.healthHandler).Methods("GET")
	r.HandleFunc("/api/entries", app.getEntriesHandler).Methods("GET")
	r.HandleFunc("/api/entries", app.createEntryHandler).Methods("POST")
	r.HandleFunc("/api/stats", app.statsHandler).Methods("GET")

	// Admin fault routes (protected by FAULTS_TOKEN)
	r.HandleFunc("/api/admin/faults/latency/enable", app.enableLatencyFault).Methods("POST")
	r.HandleFunc("/api/admin/faults/latency/disable", app.disableLatencyFault).Methods("POST")

	r.HandleFunc("/api/admin/faults/http500/enable", app.enableHTTP500Fault).Methods("POST")
	r.HandleFunc("/api/admin/faults/http500/disable", app.disableHTTP500Fault).Methods("POST")

	r.HandleFunc("/api/admin/faults/db_down/enable", app.enableDBDownFault).Methods("POST")
	r.HandleFunc("/api/admin/faults/db_down/disable", app.disableDBDownFault).Methods("POST")

	r.HandleFunc("/api/admin/faults/db_lock/enable", app.enableDBLockFault).Methods("POST")

	r.HandleFunc("/api/admin/faults/hang/enable", app.enableHangFault).Methods("POST")
	r.HandleFunc("/api/admin/faults/hang/disable", app.disableHangFault).Methods("POST")
	
	r.HandleFunc("/api/admin/faults/crash", app.crashBackend).Methods("POST")
	r.HandleFunc("/api/admin/faults/reset", app.resetFaults).Methods("POST")
	r.HandleFunc("/api/admin/faults/status", app.faultStatus).Methods("GET")
	
	port := getEnv("PORT", "8080")
	log.Printf("🚀 Server startar på port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func (app *App) initDB() {
	query := `
	CREATE TABLE IF NOT EXISTS entries (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		message TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`
	
	_, err := app.DB.Exec(query)
	if err != nil {
		log.Fatal("Kunde inte skapa tabell:", err)
	}
	log.Println("✓ Databas-schema klart")
}

func (app *App) healthHandler(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status": "healthy",
		"time":   time.Now(),
	}
	
	if err := app.DB.Ping(); err != nil {
		health["database"] = "unhealthy"
		health["status"] = "degraded"
	} else {
		health["database"] = "healthy"
	}
	
	if _, err := app.Redis.Ping(app.Ctx).Result(); err != nil {
		health["redis"] = "unhealthy"
	} else {
		health["redis"] = "healthy"
	}
	
	json.NewEncoder(w).Encode(health)
}

func (app *App) getEntriesHandler(w http.ResponseWriter, r *http.Request) {
	cacheKey := "entries:all"
	
	if app.Redis != nil {
		cached, err := app.Redis.Get(app.Ctx, cacheKey).Result()
		if err == nil {
			log.Println("✓ Cache hit")
			w.Header().Set("X-Cache", "HIT")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(cached))
			return
		}
	}
	
	// Hämta från databas
	rows, err := app.DB.Query(`
		SELECT id, name, message, created_at 
		FROM entries 
		ORDER BY created_at DESC 
		LIMIT 100
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	
	entries := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Name, &e.Message, &e.CreatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	
	// Cacha resultatet
	if app.Redis != nil {
		jsonData, _ := json.Marshal(entries)
		app.Redis.Set(app.Ctx, cacheKey, jsonData, 30*time.Second)
	}
	
	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (app *App) createEntryHandler(w http.ResponseWriter, r *http.Request) {
	var entry Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "Ogiltig data", http.StatusBadRequest)
		return
	}
	
	// Validering
	if entry.Name == "" || entry.Message == "" {
		http.Error(w, "Namn och meddelande krävs", http.StatusBadRequest)
		return
	}
	
	// Spara i databas
	err := app.DB.QueryRow(`
		INSERT INTO entries (name, message) 
		VALUES ($1, $2) 
		RETURNING id, created_at
	`, entry.Name, entry.Message).Scan(&entry.ID, &entry.CreatedAt)
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Invalidera cache
	if app.Redis != nil {
		app.Redis.Del(app.Ctx, "entries:all")
		// Incrementera statistik
		app.Redis.Incr(app.Ctx, "stats:total_entries")
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(entry)
}

func (app *App) statsHandler(w http.ResponseWriter, r *http.Request) {
	stats := make(map[string]interface{})
	
	// Räkna från databas
	var count int
	app.DB.QueryRow("SELECT COUNT(*) FROM entries").Scan(&count)
	stats["total_entries_db"] = count
	
	// Hämta från Redis om tillgängligt
	if app.Redis != nil {
		cacheCount, _ := app.Redis.Get(app.Ctx, "stats:total_entries").Result()
		stats["total_entries_created"] = cacheCount
		
		// Cache statistik
		info, _ := app.Redis.Info(app.Ctx, "stats").Result()
		if info != "" {
			stats["cache_available"] = true
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// ----------------------
// Fault middleware + admin handlers
// ----------------------

func (app *App) faultsAllowed(w http.ResponseWriter, r *http.Request) bool {
	if !app.FaultsEnabled {
		http.NotFound(w, r)
		return false
	}
	if app.FaultsToken == "" {
		http.Error(w, "Faults enabled but FAULTS_TOKEN is missing", http.StatusInternalServerError)
		return false
	}

	auth := r.Header.Get("Authorization")
	want := "Bearer " + app.FaultsToken
	if auth != want {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func (app *App) faultMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If not enabled, do nothing
		if !app.FaultsEnabled {
			next.ServeHTTP(w, r)
			return
		}

		now := time.Now()
		path := r.URL.Path

		// Read current fault state
		app.faultMu.RLock()
		lat := app.faults.Latency
		e500 := app.faults.HTTP500
		dbDown := app.faults.DBDown
		hang := app.faults.Hang
		app.faultMu.RUnlock()

		if hang.active(now) {
    		// "Permanent" hang until reset: block API calls
    		if strings.HasPrefix(path, "/api/") {
        		time.Sleep(365 * 24 * time.Hour) // effectively forever for training
        		return
 		    }
		}
		// DB down: block DB-dependent endpoints early
		// (We check for entries + stats specifically)
		if dbDown.active(now) {
			if path == "/api/entries" || path == "/api/stats" {
				http.Error(w, "DB unavailable (injected)", http.StatusServiceUnavailable)
				return
			}
		}

		// Latency injection
		if lat.active(now) && pathMatches(lat.Paths, path) && rollPercent(lat.Percent) {
			time.Sleep(time.Duration(lat.DelayMs) * time.Millisecond)
		}

		// HTTP 500 injection
		if e500.active(now) && pathMatches(e500.Paths, path) && rollPercent(e500.Percent) {
			http.Error(w, "Injected HTTP 500", http.StatusInternalServerError)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ---------- Admin: latency ----------

type enableLatencyReq struct {
	Ms              int      `json:"ms"`
	Percent         int      `json:"percent"`
	Paths           []string `json:"paths"`
	DurationSeconds int      `json:"durationSeconds"`
}

func (app *App) enableLatencyFault(w http.ResponseWriter, r *http.Request) {
	if !app.faultsAllowed(w, r) {
		return
	}

	var req enableLatencyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Ms <= 0 {
		http.Error(w, "`ms` must be > 0", http.StatusBadRequest)
		return
	}
	if req.Percent <= 0 || req.Percent > 100 {
		http.Error(w, "`percent` must be 1..100", http.StatusBadRequest)
		return
	}
	var until time.Time
	if req.DurationSeconds <= 0 {
    // "Permanent" until reset: far future
    until = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	} else {
    until = time.Now().Add(time.Duration(req.DurationSeconds) * time.Second)
}
	app.faultMu.Lock()
	app.faults.Latency = LatencyFault{
		Enabled: true,
		DelayMs: req.Ms,
		Percent: req.Percent,
		Paths:   req.Paths,
		Until:   until,
	}
	app.faultMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"until": until,
	})
}

func (app *App) disableLatencyFault(w http.ResponseWriter, r *http.Request) {
	if !app.faultsAllowed(w, r) {
		return
	}

	app.faultMu.Lock()
	app.faults.Latency = LatencyFault{}
	app.faultMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// ---------- Admin: http500 ----------

type enableHTTP500Req struct {
	Percent         int      `json:"percent"`
	Paths           []string `json:"paths"`
	DurationSeconds int      `json:"durationSeconds"`
}

func (app *App) enableHTTP500Fault(w http.ResponseWriter, r *http.Request) {
	if !app.faultsAllowed(w, r) {
		return
	}

	var req enableHTTP500Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Percent <= 0 || req.Percent > 100 {
		http.Error(w, "`percent` must be 1..100", http.StatusBadRequest)
		return
	}
	var until time.Time
	if req.DurationSeconds <= 0 {
    until = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	} else {
    until = time.Now().Add(time.Duration(req.DurationSeconds) * time.Second)
}

	app.faultMu.Lock()
	app.faults.HTTP500 = HTTP500Fault{
		Enabled: true,
		Percent: req.Percent,
		Paths:   req.Paths,
		Until:   until,
	}
	app.faultMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"until": until,
	})
}

func (app *App) disableHTTP500Fault(w http.ResponseWriter, r *http.Request) {
	if !app.faultsAllowed(w, r) {
		return
	}

	app.faultMu.Lock()
	app.faults.HTTP500 = HTTP500Fault{}
	app.faultMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// ---------- Admin: db_down ----------

type enableDBDownReq struct {
	DurationSeconds int `json:"durationSeconds"`
}

func (app *App) enableDBDownFault(w http.ResponseWriter, r *http.Request) {
	if !app.faultsAllowed(w, r) {
		return
	}

	var req enableDBDownReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	var until time.Time
	if req.DurationSeconds <= 0 {
    until = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	} else {
    until = time.Now().Add(time.Duration(req.DurationSeconds) * time.Second)
	}

	app.faultMu.Lock()
	app.faults.DBDown = TimedToggle{Enabled: true, Until: until}
	app.faultMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"until": until,
	})
}

func (app *App) disableDBDownFault(w http.ResponseWriter, r *http.Request) {
	if !app.faultsAllowed(w, r) {
		return
	}

	app.faultMu.Lock()
	app.faults.DBDown = TimedToggle{}
	app.faultMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// ---------- Admin: db_lock ----------

type enableDBLockReq struct {
	Seconds int `json:"seconds"`
}

func (app *App) enableDBLockFault(w http.ResponseWriter, r *http.Request) {
	if !app.faultsAllowed(w, r) {
		return
	}

	var req enableDBLockReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	var until time.Time
	if req.DurationSeconds <= 0 {
		until = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	} else {
		until = time.Now().Add(time.Duration(req.DurationSeconds) * time.Second)
	}

	// Mark as active for status endpoint
	app.faultMu.Lock()
	app.faults.DBLock = TimedToggle{Enabled: true, Until: until}
	app.faultMu.Unlock()

	// Start lock in background
	go func(lockSeconds int) {
		tx, err := app.DB.Begin()
		if err != nil {
			log.Println("db_lock: begin failed:", err)
			return
		}
		defer tx.Rollback()

		// Lock the table, then sleep inside the transaction
		if _, err := tx.Exec(`LOCK TABLE entries IN ACCESS EXCLUSIVE MODE`); err != nil {
			log.Println("db_lock: lock failed:", err)
			return
		}
		if _, err := tx.Exec(`SELECT pg_sleep($1)`, lockSeconds); err != nil {
			log.Println("db_lock: sleep failed:", err)
			return
		}
		_ = tx.Commit()
	}(req.Seconds)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"until": until,
	})
}

// ---------- Admin: hang ----------

type enableHangReq struct {
    DurationSeconds int `json:"durationSeconds"`
}

func (app *App) enableHangFault(w http.ResponseWriter, r *http.Request) {
    if !app.faultsAllowed(w, r) { return }

    var req enableHangReq
    _ = json.NewDecoder(r.Body).Decode(&req) // tillåt tom body

    var until time.Time
    if req.DurationSeconds <= 0 {
        until = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
    } else {
        until = time.Now().Add(time.Duration(req.DurationSeconds) * time.Second)
    }

    app.faultMu.Lock()
    app.faults.Hang = TimedToggle{Enabled: true, Until: until}
    app.faultMu.Unlock()

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]any{"ok": true, "until": until})
}

func (app *App) disableHangFault(w http.ResponseWriter, r *http.Request) {
    if !app.faultsAllowed(w, r) { return }

    app.faultMu.Lock()
    app.faults.Hang = TimedToggle{}
    app.faultMu.Unlock()

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// ---------- Admin: crash + reset + status ----------

func (app *App) crashBackend(w http.ResponseWriter, r *http.Request) {
	if !app.faultsAllowed(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "Crashing backend (injected)"})

	go func() {
		time.Sleep(200 * time.Millisecond)
		os.Exit(1)
	}()
}

func (app *App) resetFaults(w http.ResponseWriter, r *http.Request) {
	if !app.faultsAllowed(w, r) {
		return
	}

	app.faultMu.Lock()
	app.faults = FaultState{}
	app.faultMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (app *App) faultStatus(w http.ResponseWriter, r *http.Request) {
	if !app.faultsAllowed(w, r) {
		return
	}

	now := time.Now()

	app.faultMu.RLock()
	state := app.faults
	app.faultMu.RUnlock()

	resp := map[string]any{
		"time": now,
		"latency": map[string]any{
			"active":  state.Latency.active(now),
			"enabled": state.Latency.Enabled,
			"ms":      state.Latency.DelayMs,
			"percent": state.Latency.Percent,
			"paths":   state.Latency.Paths,
			"until":   state.Latency.Until,
		},
		"http500": map[string]any{
			"active":  state.HTTP500.active(now),
			"enabled": state.HTTP500.Enabled,
			"percent": state.HTTP500.Percent,
			"paths":   state.HTTP500.Paths,
			"until":   state.HTTP500.Until,
		},
		"db_down": map[string]any{
			"active":  state.DBDown.active(now),
			"enabled": state.DBDown.Enabled,
			"until":   state.DBDown.Until,
		},
		"db_lock": map[string]any{
			"active":  state.DBLock.active(now),
			"enabled": state.DBLock.Enabled,
			"until":   state.DBLock.Until,
		},
		"hang": map[string]any{
  			"active": state.Hang.active(now),
			"enabled": state.Hang.Enabled,
			"until": state.Hang.Until,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
