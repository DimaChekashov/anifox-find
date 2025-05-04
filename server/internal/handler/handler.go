package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DimaChekashov/anifox-find/internal/models"
)

func getAnimePaginated(db *sql.DB, offset, limit int) ([]models.Anime, error) {
	rows, err := db.Query("SELECT * FROM anime ORDER BY id LIMIT $1 OFFSET $2", limit, offset)
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

func HandleAnimeList(db *sql.DB) http.HandlerFunc {
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

func HandleSingleAnime(db *sql.DB) http.HandlerFunc {
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

func HandleSearchAnime(db *sql.DB) http.HandlerFunc {
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
