package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// Cursor represents pagination cursor data
type Cursor struct {
	LastPublishedAt time.Time `json:"p"`
	LastID          string    `json:"i"`
	Direction       string    `json:"d,omitempty"` // "next" or "prev"
}

// EncodeCursor creates a base64-encoded cursor string
func EncodeCursor(publishedAt time.Time, id string) string {
	cursor := Cursor{
		LastPublishedAt: publishedAt,
		LastID:          id,
		Direction:       "next",
	}

	data, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}

	return base64.URLEncoding.EncodeToString(data)
}

// DecodeCursor decodes a cursor string back to cursor data
func DecodeCursor(cursorStr string) (*Cursor, error) {
	if cursorStr == "" {
		return nil, nil
	}

	data, err := base64.URLEncoding.DecodeString(cursorStr)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor format")
	}

	var cursor Cursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, fmt.Errorf("invalid cursor data")
	}

	return &cursor, nil
}
