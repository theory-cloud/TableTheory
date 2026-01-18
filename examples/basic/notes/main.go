package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/google/uuid"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

// Note represents a note with tags and user association
// This example adds indexes and more complex data types
type Note struct {
	// Primary key
	ID string `theorydb:"pk"`

	// Global Secondary Index for querying by user
	UserID string `theorydb:"index:gsi-user,pk"`

	// Required fields
	Title   string `theorydb:"required"`
	Content string `theorydb:"required"`

	// Tags demonstrate working with sets
	Tags []string `theorydb:"set"`

	// Category for another index
	Category string `theorydb:"index:gsi-category,pk"`

	// Timestamps with index on CreatedAt for time-based queries
	CreatedAt time.Time `theorydb:"index:gsi-user,sk"`
	UpdatedAt time.Time

	// Additional metadata
	WordCount int
	IsPinned  bool
}

// NotesApp manages note operations
type NotesApp struct {
	db     core.ExtendedDB
	userID string // Current user for the demo
}

// NewNotesApp creates a new notes application
func NewNotesApp(userID string) (*NotesApp, error) {
	// Configure TableTheory
	cfg := &session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
		},
	}

	// Create TableTheory client
	db, err := theorydb.New(*cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create TableTheory client: %v", err)
	}

	// Create table with GSIs if it doesn't exist
	if err := db.CreateTable(&Note{}); err != nil {
		if !strings.Contains(err.Error(), "ResourceInUseException") {
			return nil, fmt.Errorf("failed to create table: %v", err)
		}
	}

	return &NotesApp{
		db:     db,
		userID: userID,
	}, nil
}

// Create adds a new note
func (app *NotesApp) Create(title, content, category string, tags []string) (*Note, error) {
	// Count words in content
	wordCount := len(strings.Fields(content))

	note := &Note{
		ID:        uuid.New().String(),
		UserID:    app.userID,
		Title:     title,
		Content:   content,
		Category:  category,
		Tags:      tags,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		WordCount: wordCount,
		IsPinned:  false,
	}

	if err := app.db.Model(note).Create(); err != nil {
		return nil, fmt.Errorf("failed to create note: %v", err)
	}

	fmt.Printf("✅ Created note: %s\n", note.Title)
	return note, nil
}

// ListMyNotes gets all notes for the current user
func (app *NotesApp) ListMyNotes(limit int) ([]Note, error) {
	var notes []Note

	// Query using GSI to get user's notes, sorted by creation time
	query := app.db.Model(&Note{}).
		Index("gsi-user").
		Where("UserID", "=", app.userID).
		Limit(limit)

	if err := query.All(&notes); err != nil {
		return nil, fmt.Errorf("failed to list notes: %v", err)
	}

	return notes, nil
}

// ListByCategory gets notes in a specific category
func (app *NotesApp) ListByCategory(category string) ([]Note, error) {
	var notes []Note

	// Query using category GSI
	query := app.db.Model(&Note{}).
		Index("gsi-category").
		Where("Category", "=", category)

	if err := query.All(&notes); err != nil {
		return nil, fmt.Errorf("failed to list notes by category: %v", err)
	}

	return notes, nil
}

// SearchByTag finds notes containing a specific tag
func (app *NotesApp) SearchByTag(tag string) ([]Note, error) {
	// Since we can't query directly on sets, we need to scan and filter
	var allNotes []Note

	// Get all user's notes first
	if err := app.db.Model(&Note{}).
		Index("gsi-user").
		Where("UserID", "=", app.userID).
		All(&allNotes); err != nil {
		return nil, fmt.Errorf("failed to search notes: %v", err)
	}

	// Filter by tag
	var filteredNotes []Note
	for _, note := range allNotes {
		for _, noteTag := range note.Tags {
			if strings.EqualFold(noteTag, tag) {
				filteredNotes = append(filteredNotes, note)
				break
			}
		}
	}

	return filteredNotes, nil
}

// GetRecentNotes gets notes created in the last N days
func (app *NotesApp) GetRecentNotes(days int) ([]Note, error) {
	var notes []Note

	// Calculate cutoff time
	cutoff := time.Now().AddDate(0, 0, -days)

	// Query all user's notes
	if err := app.db.Model(&Note{}).
		Index("gsi-user").
		Where("UserID", "=", app.userID).
		All(&notes); err != nil {
		return nil, fmt.Errorf("failed to get recent notes: %v", err)
	}

	// Filter by creation time (in production, use sort key for this)
	var recentNotes []Note
	for _, note := range notes {
		if note.CreatedAt.After(cutoff) {
			recentNotes = append(recentNotes, note)
		}
	}

	return recentNotes, nil
}

// Update modifies an existing note
func (app *NotesApp) Update(id string, updates map[string]any) error {
	// Get the note first
	var note Note
	if err := app.db.Model(&Note{}).Where("ID", "=", id).First(&note); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("note not found")
		}
		return fmt.Errorf("failed to get note: %v", err)
	}

	// Verify ownership
	if note.UserID != app.userID {
		return fmt.Errorf("note not found") // Don't reveal it exists
	}

	// Apply updates
	if title, ok := updates["title"].(string); ok && title != "" {
		note.Title = title
	}

	if content, ok := updates["content"].(string); ok && content != "" {
		note.Content = content
		note.WordCount = len(strings.Fields(content))
	}

	if category, ok := updates["category"].(string); ok {
		note.Category = category
	}

	if tags, ok := updates["tags"].([]string); ok {
		note.Tags = tags
	}

	if pinned, ok := updates["pinned"].(bool); ok {
		note.IsPinned = pinned
	}

	note.UpdatedAt = time.Now()

	// Save changes
	if err := app.db.Model(&note).Update(); err != nil {
		return fmt.Errorf("failed to update note: %v", err)
	}

	fmt.Println("✅ Note updated successfully")
	return nil
}

// Delete removes a note
func (app *NotesApp) Delete(id string) error {
	// Verify ownership first
	var note Note
	if err := app.db.Model(&Note{}).Where("ID", "=", id).First(&note); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("note not found")
		}
		return fmt.Errorf("failed to get note: %v", err)
	}

	if note.UserID != app.userID {
		return fmt.Errorf("note not found")
	}

	// Delete the note
	if err := app.db.Model(&Note{}).Where("ID", "=", id).Delete(); err != nil {
		return fmt.Errorf("failed to delete note: %v", err)
	}

	fmt.Println("✅ Note deleted successfully")
	return nil
}

// GetStats returns statistics about user's notes
func (app *NotesApp) GetStats() (map[string]any, error) {
	notes, err := app.ListMyNotes(1000) // Get all notes
	if err != nil {
		return nil, err
	}

	// Calculate statistics
	totalWords := 0
	categories := make(map[string]int)
	tags := make(map[string]int)
	pinnedCount := 0

	for _, note := range notes {
		totalWords += note.WordCount
		categories[note.Category]++

		if note.IsPinned {
			pinnedCount++
		}

		for _, tag := range note.Tags {
			tags[tag]++
		}
	}

	return map[string]any{
		"total_notes":    len(notes),
		"total_words":    totalWords,
		"pinned_count":   pinnedCount,
		"categories":     categories,
		"popular_tags":   tags,
		"avg_word_count": totalWords / max(len(notes), 1),
	}, nil
}

// CLI functions

func (app *NotesApp) printNotes(notes []Note, title string) {
	if len(notes) == 0 {
		fmt.Printf("No notes found%s.\n", title)
		return
	}

	fmt.Printf("\n📝 %s (%d notes):\n", title, len(notes))
	fmt.Println("───────────────────────")

	for i, note := range notes {
		pin := ""
		if note.IsPinned {
			pin = "📌 "
		}

		// Format tags
		tagStr := ""
		if len(note.Tags) > 0 {
			tagStr = fmt.Sprintf(" [%s]", strings.Join(note.Tags, ", "))
		}

		// Show preview of content
		preview := note.Content
		if len(preview) > 50 {
			preview = preview[:47] + "..."
		}

		fmt.Printf("%d. %s%s (%s)%s\n", i+1, pin, note.Title, note.Category, tagStr)
		fmt.Printf("   %s\n", preview)
		fmt.Printf("   📅 %s | 📝 %d words | ID: %s\n\n",
			note.CreatedAt.Format("2006-01-02"),
			note.WordCount,
			note.ID[:8])
	}
}

func (app *NotesApp) runCLI() {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Printf("🗒️  Welcome to TableTheory Notes App!\n")
	fmt.Printf("Logged in as: %s\n\n", app.userID)

	// Show initial notes
	if notes, err := app.ListMyNotes(10); err == nil {
		app.printNotes(notes, "Your Recent Notes")
	}

	fmt.Println("\nCommands:")
	fmt.Println("  add                    - Create a new note")
	fmt.Println("  list [limit]          - List your notes")
	fmt.Println("  category <name>       - List notes by category")
	fmt.Println("  tag <tag>             - Search notes by tag")
	fmt.Println("  recent <days>         - Show notes from last N days")
	fmt.Println("  pin <num>             - Toggle pin status")
	fmt.Println("  delete <num>          - Delete a note")
	fmt.Println("  stats                 - Show statistics")
	fmt.Println("  quit                  - Exit")
	fmt.Println()

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		parts := strings.Fields(input)

		if len(parts) == 0 {
			continue
		}

		command := parts[0]

		switch command {
		case "add":
			app.handleAdd(scanner)

		case "list":
			limit := 10
			if len(parts) > 1 {
				if l, err := strconv.Atoi(parts[1]); err == nil {
					limit = l
				}
			}
			if notes, err := app.ListMyNotes(limit); err == nil {
				app.printNotes(notes, "Your Notes")
			} else {
				fmt.Printf("Error: %v\n", err)
			}

		case "category":
			if len(parts) < 2 {
				fmt.Println("Usage: category <name>")
				continue
			}
			category := strings.Join(parts[1:], " ")
			if notes, err := app.ListByCategory(category); err == nil {
				app.printNotes(notes, fmt.Sprintf("Notes in '%s'", category))
			} else {
				fmt.Printf("Error: %v\n", err)
			}

		case "tag":
			if len(parts) < 2 {
				fmt.Println("Usage: tag <tag>")
				continue
			}
			tag := parts[1]
			if notes, err := app.SearchByTag(tag); err == nil {
				app.printNotes(notes, fmt.Sprintf("Notes tagged '%s'", tag))
			} else {
				fmt.Printf("Error: %v\n", err)
			}

		case "recent":
			days := 7
			if len(parts) > 1 {
				if d, err := strconv.Atoi(parts[1]); err == nil {
					days = d
				}
			}
			if notes, err := app.GetRecentNotes(days); err == nil {
				app.printNotes(notes, fmt.Sprintf("Notes from last %d days", days))
			} else {
				fmt.Printf("Error: %v\n", err)
			}

		case "pin":
			if len(parts) < 2 {
				fmt.Println("Usage: pin <number>")
				continue
			}
			app.handlePin(parts[1])

		case "delete":
			if len(parts) < 2 {
				fmt.Println("Usage: delete <number>")
				continue
			}
			app.handleDelete(parts[1])

		case "stats":
			if stats, err := app.GetStats(); err == nil {
				app.printStats(stats)
			} else {
				fmt.Printf("Error: %v\n", err)
			}

		case "quit", "exit":
			fmt.Println("👋 Goodbye!")
			return

		default:
			fmt.Println("Unknown command. Try: add, list, category, tag, recent, pin, delete, stats, quit")
		}
	}
}

func (app *NotesApp) handleAdd(scanner *bufio.Scanner) {
	fmt.Print("Title: ")
	scanner.Scan()
	title := strings.TrimSpace(scanner.Text())

	fmt.Print("Category (personal/work/ideas/other): ")
	scanner.Scan()
	category := strings.TrimSpace(scanner.Text())
	if category == "" {
		category = "other"
	}

	fmt.Print("Content: ")
	scanner.Scan()
	content := strings.TrimSpace(scanner.Text())

	fmt.Print("Tags (comma-separated): ")
	scanner.Scan()
	tagInput := strings.TrimSpace(scanner.Text())

	var tags []string
	if tagInput != "" {
		for _, tag := range strings.Split(tagInput, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	if _, err := app.Create(title, content, category, tags); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func (app *NotesApp) handlePin(numStr string) error {
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return fmt.Errorf("invalid number")
	}

	notes, err := app.ListMyNotes(100)
	if err != nil {
		return err
	}

	if num < 1 || num > len(notes) {
		return fmt.Errorf("invalid note number")
	}

	note := notes[num-1]
	return app.Update(note.ID, map[string]any{
		"pinned": !note.IsPinned,
	})
}

func (app *NotesApp) handleDelete(numStr string) error {
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return fmt.Errorf("invalid number")
	}

	notes, err := app.ListMyNotes(100)
	if err != nil {
		return err
	}

	if num < 1 || num > len(notes) {
		return fmt.Errorf("invalid note number")
	}

	return app.Delete(notes[num-1].ID)
}

func (app *NotesApp) printStats(stats map[string]any) {
	fmt.Println("\n📊 Your Notes Statistics:")
	fmt.Println("────────────────────────")
	fmt.Printf("Total Notes: %v\n", stats["total_notes"])
	fmt.Printf("Total Words: %v\n", stats["total_words"])
	fmt.Printf("Average Words/Note: %v\n", stats["avg_word_count"])
	fmt.Printf("Pinned Notes: %v\n", stats["pinned_count"])

	if categories, ok := stats["categories"].(map[string]int); ok && len(categories) > 0 {
		fmt.Println("\nCategories:")
		for cat, count := range categories {
			fmt.Printf("  - %s: %d notes\n", cat, count)
		}
	}

	if tags, ok := stats["popular_tags"].(map[string]int); ok && len(tags) > 0 {
		fmt.Println("\nPopular Tags:")
		for tag, count := range tags {
			fmt.Printf("  - #%s (%d)\n", tag, count)
		}
	}
	fmt.Println()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	// Get username from environment or use default
	userID := os.Getenv("USER")
	if userID == "" {
		userID = "demo-user"
	}

	app, err := NewNotesApp(userID)
	if err != nil {
		log.Fatalf("Failed to initialize app: %v", err)
	}

	app.runCLI()
}
