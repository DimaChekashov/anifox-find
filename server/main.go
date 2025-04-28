package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type AnimeClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewAnimeClient() *AnimeClient {
	return &AnimeClient{
		baseURL: "https://api.jikan.moe/v4",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type Anime struct {
	ID       int    `json:"mal_id"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Image    string `json:"image"`
	Episodes int    `json:"episodes"`
	Aired    Aired  `json:"aired"`
	Synopsis string `json:"synopsis"`
}

type Aired struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type ImageFormats struct {
	JPG struct {
		ImageURL      string `json:"image_url"`
		SmallImageURL string `json:"small_image_url"`
		LargeImageURL string `json:"large_image_url"`
	} `json:"jpg"`
	WebP struct {
		ImageURL      string `json:"image_url"`
		SmallImageURL string `json:"small_image_url"`
		LargeImageURL string `json:"large_image_url"`
	} `json:"webp"`
}

type AnimeResponse struct {
	Data struct {
		ID       int          `json:"mal_id"`
		URL      string       `json:"url"`
		Title    string       `json:"title"`
		Images   ImageFormats `json:"images"`
		Episodes int          `json:"episodes"`
		Aired    Aired        `json:"aired"`
		Synopsis string       `json:"synopsis"`
	} `json:"data"`
}

func (c *AnimeClient) GetAnimeByID(ctx context.Context, id int) (*Anime, error) {
	url := fmt.Sprintf("%s/anime/%d", c.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read body failed: %w", err)
	}

	var result AnimeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	anime := &Anime{
		ID:       result.Data.ID,
		URL:      result.Data.URL,
		Title:    result.Data.Title,
		Image:    result.Data.Images.JPG.LargeImageURL,
		Episodes: result.Data.Episodes,
		Aired:    result.Data.Aired,
		Synopsis: result.Data.Synopsis,
	}

	return anime, nil
}

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

func saveAnime(db *sql.DB, anime Anime) error {
	airedJSON, err := json.Marshal(anime.Aired)
	if err != nil {
		return fmt.Errorf("failed to marshal aired data: %w", err)
	}

	query := `
	INSERT INTO anime (
		url,
		title,
		image,
		episodes,
		aired,
		synopsis,
		updated
	) VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		url = excluded.url,
		title = excluded.title,
		image = excluded.image,
		episodes = excluded.episodes,
		aired = excluded.aired,
		synopsis = excluded.synopsis,
		updated = excluded.updated`

	_, err = db.Exec(query,
		anime.URL,
		anime.Title,
		anime.Image,
		anime.Episodes,
		airedJSON,
		anime.Synopsis,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("failed to save anime (ID: %d): %w", anime.ID, err)
	}

	return nil
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

func parseAnimeAndSaveToDB(db *sql.DB, size int) error {
	client := NewAnimeClient()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var successCount, failCount int

	for i := 1; i <= size; i++ {
		if i > 1 {
			time.Sleep(350 * time.Millisecond)
		}

		anime, err := client.GetAnimeByID(ctx, i)
		if err != nil {
			log.Printf("⚠️ Anime with ID %d not found (404) or error: %v", i, err)
			failCount++
			continue
		}
		if err := saveAnime(db, *anime); err != nil {
			log.Printf("Error saving anime: %d: %v", anime.ID, err)
		} else {
			log.Printf("Success saving anime: %s", anime.Title)
		}

		successCount++
		log.Printf("Success got anime %d: %s", i, anime.Title)
	}

	return nil
}

func getAnimePaginated(db *sql.DB, offset, limit int) ([]Anime, error) {
	rows, err := db.Query("SELECT * FROM anime ORDER BY id LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var animeList []Anime
	var updated time.Time

	for rows.Next() {
		var a Anime
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
		w.Write(jsonData)
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

	server := http.Server{
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("Server start on http://localhost%s", server.Addr)
	log.Fatal(server.ListenAndServe())

	// parseAnimeAndSaveToDB(db, 60)
}
