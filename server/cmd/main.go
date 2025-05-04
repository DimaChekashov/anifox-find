package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/DimaChekashov/anifox-find/internal/handler"
	"github.com/DimaChekashov/anifox-find/internal/models"
	_ "github.com/lib/pq"
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
	connStr := "postgres://postgres:qwerty@localhost:5432/anifox?sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("error opening DB: %w", err)
	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("error connecting to DB: %w", err)
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

func main() {
	db, err := initDB()
	if err != nil {
		log.Fatalf("Error init DB: %v", err)
	}
	defer db.Close()

	http.HandleFunc("/anime", handler.HandleAnimeList(db))
	http.HandleFunc("/anime/", handler.HandleSingleAnime(db))
	http.HandleFunc("/anime/search", handler.HandleSearchAnime(db))

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
