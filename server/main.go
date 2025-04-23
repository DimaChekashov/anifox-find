package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mattn/go-sqlite3"
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
	ID       int     `json:"mal_id"`
	URL      string  `json:"url"`
	Title    string  `json:"title"`
	Images   Images  `json:"images"`
	Episodes int     `json:"episodes"`
	Score    float64 `json:"score"`
	Aired    Aired   `json:"aired"`
	Genres   []Genre `json:"genres"`
	Synopsis string  `json:"synopsis"`
}

type Images struct {
	JPG ImageURLs `json:"jpg"`
}

type ImageURLs struct {
	ImageURL      string `json:"image_url"`
	SmallImageURL string `json:"small_image_url"`
	LargeImageURL string `json:"large_image_url"`
}

type Aired struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type Genre struct {
	ID   int    `json:"mal_id"`
	Name string `json:"name"`
}

type AnimeResponse struct {
	Data Anime `json:"data"`
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

	return &result.Data, nil
}

func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "anime.db")
	if err != nil {
		return nil, fmt.Errorf("Error opening DB: %w", err)
	}

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS anime (
		id INTEGER PRIMARY KEY,
		url TEXT,
		title TEXT NOT NULL,
		images TEXT,
		episodes INTEGER.
		score REAL,
		aired TEXT,
		genres TEXT,
		synopsis TEXT,
		updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return nil, fmt.Errorf("Error creating table: %w", err)
	}

	return db, nil
}

func saveAnime(db *sql.DB, anime Anime) error {
	imagesJSON, err := json.Marshal(anime.Images)
	if err != nil {
		return fmt.Errorf("Error encoding images: %w", err)
	}

	airedJSON, err := json.Marshal(anime.Aired)
	if err != nil {
		return fmt.Errorf("Error encoding aired: %w", err)
	}

	genresJSON, err := json.Marshal(anime.Genres)
	if err != nil {
		return fmt.Errorf("Error encoding genres: %w", err)
	}

	_, err = db.Exec(`
	INSERT INTO anime (id, url, title, images, episodes, score, aired, genres, synopsis, updated)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		url = excluded.url,
		title = excluded.title,
		images = excluded.images,
		episodes = excluded.episodes,
		score = excluded.score,
		aired = excluded.aired,
		genres = excluded.genres,
		synopsis = excluded.synopsis,
		updated = excluded.updated`,
		anime.ID,
		anime.URL,
		anime.Title,
		string(imagesJSON),
		anime.Episodes,
		anime.Score,
		string(airedJSON),
		string(genresJSON),
		anime.Synopsis,
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("Error saving into DB: %w", err)
	}

	return nil
}

func main() {
	client := NewAnimeClient()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	animeList := []Anime{}
	var successCount, failCount int

	for i := 1; i <= 20; i++ {
		if i > 1 {
			time.Sleep(350 * time.Millisecond)
		}

		anime, err := client.GetAnimeByID(ctx, i)
		if err != nil {
			log.Printf("⚠️ Anime with ID %d not found (404) or error: %v", i, err)
			failCount++
			continue
		}

		animeList = append(animeList, *anime)
		successCount++
		log.Printf("Success got anime %d: %s", i, anime.Title)
	}

	log.Printf("Result: success %d, not found %d", successCount, failCount)

	if successCount == 0 {
		log.Fatal("Can not get anyone anime")
	}

	data, err := json.MarshalIndent(animeList, "", " ")
	if err != nil {
		log.Fatalf("Error json encoding: %v", err)
	}

	err = os.WriteFile("anime.json", data, 0644)
	if err != nil {
		log.Fatalf("Error writing to file: %v", err)
	}

	log.Println("The data successfully writen into anime.json")
}
