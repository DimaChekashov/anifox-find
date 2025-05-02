package models

import "time"

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
