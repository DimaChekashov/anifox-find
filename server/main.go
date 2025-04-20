package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
	ID       int `json:""`
	URL      string
	Title    string
	Images   Images
	Episodes int
	Score    float64
	Aired    Aired
	Genres   []Genre
	Synopsis string
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	anime, err := client.GetAnimeByID(ctx, 1)
	if err != nil {
		log.Fatalf("Error getting anime: %v", err)
	}

	fmt.Print(anime)
}
