package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
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
