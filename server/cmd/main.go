package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DimaChekashov/anifox-find/internal/models"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
	// "github.com/DimaChekashov/anifox-find/internal/parser"
)

// User repository
var ErrUserNotFound = errors.New("user not found")

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, u *models.User) error {
	query := `
		INSERT INTO users (username, email, password, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`

	return r.db.QueryRowContext(
		ctx,
		query,
		u.Username,
		u.Email,
		u.Password,
		"user",
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	u := &models.User{}
	query := `SELECT id, username, email, password, role FROM users WHERE username = $1`
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&u.ID,
		&u.Username,
		&u.Email,
		&u.Password,
		&u.Role,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}

	return u, err
}

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) RegisterUser(ctx context.Context, req *models.AuthRequest) (*models.User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Username: req.Username,
		Email:    req.Email,
		Password: string(hashedPassword),
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) Authenticate(ctx context.Context, username, password string) (*models.User, error) {
	user, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}

	return user, nil
}

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req models.AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	user, err := h.svc.RegisterUser(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// DB
func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "anime.db")
	if err != nil {
		return nil, fmt.Errorf("error opening DB: %w", err)
	}

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS anime (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT,
		title TEXT NOT NULL,
		image TEXT,
		episodes INTEGER,
		aired TEXT,
		synopsis TEXT,
		updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return nil, fmt.Errorf("error creating table: %w", err)
	}

	return db, nil
}

func exportAnimeToJSON(db *sql.DB) ([]byte, error) {
	rows, err := db.Query("SELECT * FROM anime")
	if err != nil {
		return nil, fmt.Errorf("error query: %v", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("error get column: %v", err)
	}

	var results []map[string]interface{}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}

		if err := rows.Scan(pointers...); err != nil {
			return nil, fmt.Errorf("error scan rows: %v", err)
		}

		rowData := make(map[string]interface{})
		for i, colName := range columns {
			val := values[i]

			switch v := val.(type) {
			case []byte:
				var jsonData interface{}
				if err := json.Unmarshal(v, &jsonData); err == nil {
					rowData[colName] = jsonData
				} else {
					rowData[colName] = string(v)
				}
			case nil:
				rowData[colName] = nil
			default:
				rowData[colName] = v
			}
		}

		results = append(results, rowData)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iteration rows: %v", err)
	}

	return json.MarshalIndent(results, "", "  ")
}

func exportOneAnimeToJSONByID(db *sql.DB, id int) ([]byte, error) {
	row := db.QueryRow(`
		SELECT
			id,
			url,
			title,
			image,
			episodes,
			aired,
			synopsis,
			updated
		FROM anime
		WHERE ID = ?
	`, id)

	columns := []string{"id", "url", "title", "image", "episodes", "aired", "synopsis", "updated"}

	values := make([]interface{}, len(columns))
	pointers := make([]interface{}, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}

	if err := row.Scan(pointers...); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("anime with ID %d not found", id)
		}
		return nil, fmt.Errorf("database scan error: %w", err)
	}

	result := make(map[string]interface{})
	for i, colName := range columns {
		val := values[i]

		switch v := val.(type) {
		case []byte:
			if colName == "images" || colName == "genres" {
				var jsonData interface{}
				if err := json.Unmarshal(v, &jsonData); err == nil {
					result[colName] = jsonData
					continue
				}
			}
			result[colName] = string(v)
		case nil:
			result[colName] = nil
		default:
			result[colName] = v
		}
	}

	return json.MarshalIndent(result, "", "  ")
}

func getAnimePaginated(db *sql.DB, offset, limit int) ([]models.Anime, error) {
	rows, err := db.Query("SELECT * FROM anime ORDER BY id LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var animeList []models.Anime
	var updated time.Time

	for rows.Next() {
		var a models.Anime
		var airedJSON []byte
		var image sql.NullString

		err := rows.Scan(
			&a.ID,
			&a.URL,
			&a.Title,
			&image,
			&a.Episodes,
			&airedJSON,
			&a.Synopsis,
			&updated,
		)
		if err != nil {
			return nil, fmt.Errorf("row scan error: %w", err)
		}

		if image.Valid {
			a.Image = image.String
		} else {
			a.Image = ""
		}

		if len(airedJSON) > 0 {
			if err := json.Unmarshal(airedJSON, &a.Aired); err != nil {
				return nil, fmt.Errorf("failed to parse aired JSON: %w", err)
			}
		}

		animeList = append(animeList, a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return animeList, nil
}

func getTotalAnimeCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM anime").Scan(&count)
	return count, err
}

func searchAnimeByTitle(db *sql.DB, title string) ([]models.Anime, error) {
	query := `SELECT * FROM anime WHERE title LIKE ?`

	rows, err := db.Query(query, "%"+title+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var animeList []models.Anime
	var updated time.Time
	for rows.Next() {
		var a models.Anime
		var airedJSON []byte
		var image sql.NullString
		if err := rows.Scan(
			&a.ID,
			&a.URL,
			&a.Title,
			&image,
			&a.Episodes,
			&airedJSON,
			&a.Synopsis,
			&updated,
		); err != nil {
			return nil, err
		}
		animeList = append(animeList, a)
	}

	return animeList, nil
}

// Handlers
func handleAnimeList(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		page := 1
		limit := 20

		if p := r.URL.Query().Get("page"); p != "" {
			if pNum, err := strconv.Atoi(p); err == nil && pNum > 0 {
				page = pNum
			}
		}

		if l := r.URL.Query().Get("limit"); l != "" {
			if lNum, err := strconv.Atoi(l); err == nil && lNum > 0 && lNum <= 100 {
				limit = lNum
			}
		}

		offset := (page - 1) * limit

		animeList, err := getAnimePaginated(db, offset, limit)
		if err != nil {
			log.Printf("Database error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		total, err := getTotalAnimeCount(db)
		if err != nil {
			log.Printf("Count error: %v", err)
		}

		response := map[string]interface{}{
			"data": animeList,
			"meta": map[string]interface{}{
				"page":       page,
				"limit":      limit,
				"total":      total,
				"totalPages": int(math.Ceil(float64(total) / float64(limit))),
			},
		}

		jsonData, err := json.Marshal(response)
		if err != nil {
			log.Printf("JSON error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	}
}

func handleSingleAnime(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		idStr := strings.TrimPrefix(r.URL.Path, "/anime/")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid anime ID", http.StatusBadRequest)
			return
		}

		jsonData, err := exportOneAnimeToJSONByID(db, id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
			} else {
				log.Printf("Error: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonData)
	}
}

func handleSearchAnime(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		titleQuery := strings.TrimSpace(r.URL.Query().Get("title"))
		if titleQuery == "" {
			http.Error(w, "Title query parameter is required", http.StatusBadRequest)
			return
		}

		results, err := searchAnimeByTitle(db, titleQuery)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(results)
	}
}

func main() {
	db, err := initDB()
	if err != nil {
		log.Fatalf("Error init DB: %v", err)
	}
	defer db.Close()

	http.HandleFunc("/anime", handleAnimeList(db))
	http.HandleFunc("/anime/", handleSingleAnime(db))
	http.HandleFunc("/anime/search", handleSearchAnime(db))

	userRepo := NewRepository(db)
	userSvc := NewService(userRepo)
	userHandler := NewHandler(userSvc)

	http.HandleFunc("/register", userHandler.Register)

	server := http.Server{
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("Server start on http://localhost%s", server.Addr)
	log.Fatal(server.ListenAndServe())

	// parser.ParseAnimeAndSaveToDB(db, 60)
}
