package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

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

func (c *AnimeClient) getAnimeByID(ctx context.Context, id int) (*Anime, error) {
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

func ParseAnimeAndSaveToDB(db *sql.DB, size int) error {
	client := NewAnimeClient()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var successCount, failCount int

	for i := 1; i <= size; i++ {
		if i > 1 {
			time.Sleep(350 * time.Millisecond)
		}

		anime, err := client.getAnimeByID(ctx, i)
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
