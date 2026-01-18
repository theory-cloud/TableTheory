package handlers

import (
	"fmt"
	"testing"
	"time"
)

func TestEncodeCursor(t *testing.T) {
	publishedAt := time.Now()
	id := "test-post-123"

	cursor := EncodeCursor(publishedAt, id)
	if cursor == "" {
		t.Error("Expected non-empty cursor")
	}

	// Decode and verify
	decoded, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("Failed to decode cursor: %v", err)
	}

	if !decoded.LastPublishedAt.Equal(publishedAt) {
		t.Errorf("Expected published at %v, got %v", publishedAt, decoded.LastPublishedAt)
	}

	if decoded.LastID != id {
		t.Errorf("Expected ID %s, got %s", id, decoded.LastID)
	}

	if decoded.Direction != "next" {
		t.Errorf("Expected direction 'next', got %s", decoded.Direction)
	}
}

func TestDecodeCursor(t *testing.T) {
	tests := []struct {
		name      string
		cursor    string
		wantError bool
	}{
		{
			name:      "empty cursor",
			cursor:    "",
			wantError: false,
		},
		{
			name:      "invalid base64",
			cursor:    "invalid!@#$",
			wantError: true,
		},
		{
			name:      "invalid json",
			cursor:    "aW52YWxpZCBqc29u", // "invalid json" in base64
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeCursor(tt.cursor)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if tt.cursor == "" && result != nil {
					t.Error("Expected nil result for empty cursor")
				}
			}
		})
	}
}

func TestCursorRoundTrip(t *testing.T) {
	// Test multiple round trips
	for i := 0; i < 10; i++ {
		publishedAt := time.Now().Add(time.Duration(i) * time.Hour)
		id := fmt.Sprintf("post-%d", i)

		cursor := EncodeCursor(publishedAt, id)
		decoded, err := DecodeCursor(cursor)

		if err != nil {
			t.Fatalf("Round trip %d failed: %v", i, err)
		}

		if !decoded.LastPublishedAt.Equal(publishedAt) {
			t.Errorf("Round trip %d: time mismatch", i)
		}

		if decoded.LastID != id {
			t.Errorf("Round trip %d: ID mismatch", i)
		}
	}
}
