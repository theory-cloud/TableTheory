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

// Todo represents a task in our todo list
// This is the simplest possible TableTheory model
type Todo struct {
	// ID is our primary key - every DynamoDB item needs one
	ID string `theorydb:"pk"`

	// Title is required - we use the required tag to ensure it's not empty
	Title string `theorydb:"required"`

	// Completed tracks whether the task is done
	Completed bool

	// CreatedAt helps us sort todos by creation time
	CreatedAt time.Time

	// UpdatedAt tracks when the todo was last modified
	UpdatedAt time.Time
}

// TodoApp manages our todo list operations
type TodoApp struct {
	db core.DB
}

// NewTodoApp creates a new todo application instance
func NewTodoApp() (*TodoApp, error) {
	// Configure TableTheory for local DynamoDB
	cfg := &session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000", // Local DynamoDB endpoint
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
		},
	}

	// Create TableTheory client - now returns an interface
	db, err := tabletheory.NewBasic(*cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create TableTheory client: %v", err)
	}

	// For table creation, we need the extended interface
	extDB, err := tabletheory.New(*cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create TableTheory client: %v", err)
	}

	// Create table if it doesn't exist
	if err := extDB.CreateTable(&Todo{}); err != nil {
		// It's okay if table already exists
		if !strings.Contains(err.Error(), "ResourceInUseException") {
			return nil, fmt.Errorf("failed to create table: %v", err)
		}
	}

	return &TodoApp{db: db}, nil
}

// Create adds a new todo to our list
func (app *TodoApp) Create(title string) (*Todo, error) {
	// Create a new todo with generated ID and timestamps
	todo := &Todo{
		ID:        uuid.New().String(), // Generate unique ID
		Title:     title,
		Completed: false, // New todos start incomplete
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save to DynamoDB
	// The Create method validates required fields and saves the item
	if err := app.db.Model(todo).Create(); err != nil {
		return nil, fmt.Errorf("failed to create todo: %v", err)
	}

	fmt.Printf("✅ Created todo: %s\n", todo.Title)
	return todo, nil
}

// List retrieves all todos from the database
func (app *TodoApp) List() ([]Todo, error) {
	var todos []Todo

	// Scan retrieves all items from the table
	// For small datasets this is fine, but for large tables use Query with indexes
	if err := app.db.Model(&Todo{}).Scan(&todos); err != nil {
		return nil, fmt.Errorf("failed to list todos: %v", err)
	}

	// Sort by creation time (in production, use a sort key instead)
	// This is just for display purposes in our simple example
	for i := 0; i < len(todos)-1; i++ {
		for j := i + 1; j < len(todos); j++ {
			if todos[i].CreatedAt.After(todos[j].CreatedAt) {
				todos[i], todos[j] = todos[j], todos[i]
			}
		}
	}

	return todos, nil
}

// Get retrieves a single todo by ID
func (app *TodoApp) Get(id string) (*Todo, error) {
	todo := &Todo{}

	// Use Where to find by primary key
	err := app.db.Model(&Todo{}).Where("ID", "=", id).First(todo)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("todo not found")
		}
		return nil, fmt.Errorf("failed to get todo: %v", err)
	}

	return todo, nil
}

// Update modifies an existing todo
func (app *TodoApp) Update(id string, updates map[string]any) error {
	// First, check if the todo exists
	todo, err := app.Get(id)
	if err != nil {
		return err
	}

	// Apply updates to the todo object
	if title, ok := updates["title"].(string); ok && title != "" {
		todo.Title = title
	}

	if completed, ok := updates["completed"].(bool); ok {
		todo.Completed = completed
	}

	todo.UpdatedAt = time.Now()

	// Update in database
	// This performs a full item replacement
	if err := app.db.Model(todo).Update(); err != nil {
		return fmt.Errorf("failed to update todo: %v", err)
	}

	fmt.Println("✅ Todo updated successfully")
	return nil
}

// Delete removes a todo from the list
func (app *TodoApp) Delete(id string) error {
	// Delete by primary key
	err := app.db.Model(&Todo{}).Where("ID", "=", id).Delete()
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("todo not found")
		}
		return fmt.Errorf("failed to delete todo: %v", err)
	}

	fmt.Println("✅ Todo deleted successfully")
	return nil
}

// ToggleComplete flips the completed status of a todo
func (app *TodoApp) ToggleComplete(id string) error {
	// Get current todo
	todo, err := app.Get(id)
	if err != nil {
		return err
	}

	// Toggle the completed status
	return app.Update(id, map[string]any{
		"completed": !todo.Completed,
	})
}

// CLI functions for interactive demo

func (app *TodoApp) printTodos() error {
	todos, err := app.List()
	if err != nil {
		return err
	}

	if len(todos) == 0 {
		fmt.Println("No todos yet. Create one with 'add <title>'")
		return nil
	}

	fmt.Println("\n📝 Your Todos:")
	fmt.Println("─────────────")

	for i, todo := range todos {
		status := "[ ]"
		if todo.Completed {
			status = "[✓]"
		}

		// Show first 8 chars of ID for easy reference
		shortID := todo.ID
		if len(todo.ID) > 8 {
			shortID = todo.ID[:8]
		}
		fmt.Printf("%d. %s %s (ID: %s)\n", i+1, status, todo.Title, shortID)
	}

	fmt.Println()
	return nil
}

func (app *TodoApp) runCLI() {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("🚀 Welcome to TableTheory Todo App!")
	fmt.Println("This example demonstrates basic CRUD operations.")
	fmt.Println()

	// Show initial todos
	app.printTodos()

	fmt.Println("Commands:")
	fmt.Println("  add <title>     - Create a new todo")
	fmt.Println("  list           - Show all todos")
	fmt.Println("  complete <num> - Mark todo as complete")
	fmt.Println("  delete <num>   - Delete a todo")
	fmt.Println("  quit           - Exit")
	fmt.Println()

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(input, " ", 2)

		if len(parts) == 0 {
			continue
		}

		command := parts[0]

		switch command {
		case "add":
			if len(parts) < 2 {
				fmt.Println("Usage: add <title>")
				continue
			}
			if _, err := app.Create(parts[1]); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
			app.printTodos()

		case "list":
			app.printTodos()

		case "complete":
			if len(parts) < 2 {
				fmt.Println("Usage: complete <number>")
				continue
			}
			if err := app.handleToggle(parts[1]); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				app.printTodos()
			}

		case "delete":
			if len(parts) < 2 {
				fmt.Println("Usage: delete <number>")
				continue
			}
			if err := app.handleDelete(parts[1]); err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				app.printTodos()
			}

		case "quit", "exit":
			fmt.Println("👋 Goodbye!")
			return

		default:
			fmt.Println("Unknown command. Try: add, list, complete, delete, quit")
		}
	}
}

func (app *TodoApp) handleToggle(numStr string) error {
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return fmt.Errorf("invalid number")
	}

	todos, err := app.List()
	if err != nil {
		return err
	}

	if num < 1 || num > len(todos) {
		return fmt.Errorf("invalid todo number")
	}

	return app.ToggleComplete(todos[num-1].ID)
}

func (app *TodoApp) handleDelete(numStr string) error {
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return fmt.Errorf("invalid number")
	}

	todos, err := app.List()
	if err != nil {
		return err
	}

	if num < 1 || num > len(todos) {
		return fmt.Errorf("invalid todo number")
	}

	return app.Delete(todos[num-1].ID)
}

func main() {
	app, err := NewTodoApp()
	if err != nil {
		log.Fatalf("Failed to initialize app: %v", err)
	}

	// Run interactive CLI
	app.runCLI()
}
