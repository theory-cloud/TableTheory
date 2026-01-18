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

// Contact represents a contact with composite keys for organization
type Contact struct {
	// Composite primary key: OrgID#ContactID
	ID string `theorydb:"pk"`

	// Organization ID (extracted from composite key)
	OrgID string

	// Contact's unique ID
	ContactID string

	// GSI for searching by email across all orgs
	Email string `theorydb:"index:gsi-email,pk;required"`

	// GSI for searching by phone
	Phone string `theorydb:"index:gsi-phone,pk"`

	// Contact details
	FirstName string `theorydb:"required"`
	LastName  string `theorydb:"required"`
	Company   string
	JobTitle  string

	// GSI for full name search (LastName#FirstName)
	FullName string `theorydb:"index:gsi-name,pk"`

	// Address information
	Address struct {
		Street  string
		City    string
		State   string
		ZipCode string
		Country string
	}

	// Contact metadata
	Tags       []string `theorydb:"set"`
	Notes      string
	IsFavorite bool

	// Timestamps
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastContactedAt *time.Time

	// Custom fields as a map
	CustomFields map[string]string
}

// ContactsApp manages contact operations
type ContactsApp struct {
	db    core.ExtendedDB
	orgID string // Current organization
}

// NewContactsApp creates a new contacts application
func NewContactsApp(orgID string) (*ContactsApp, error) {
	cfg := &session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
		},
	}

	db, err := theorydb.New(*cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create TableTheory client: %v", err)
	}

	// Create table with multiple GSIs
	if err := db.CreateTable(&Contact{}); err != nil {
		if !strings.Contains(err.Error(), "ResourceInUseException") {
			return nil, fmt.Errorf("failed to create table: %v", err)
		}
	}

	return &ContactsApp{
		db:    db,
		orgID: orgID,
	}, nil
}

// Create adds a new contact
func (app *ContactsApp) Create(contact *Contact) error {
	// Generate IDs and composite key
	contact.ContactID = uuid.New().String()
	contact.ID = fmt.Sprintf("%s#%s", app.orgID, contact.ContactID)
	contact.OrgID = app.orgID

	// Set full name for searching
	contact.FullName = fmt.Sprintf("%s#%s", contact.LastName, contact.FirstName)

	// Set timestamps
	contact.CreatedAt = time.Now()
	contact.UpdatedAt = time.Now()

	// Validate unique email within org
	if existing, _ := app.FindByEmail(contact.Email); existing != nil {
		return fmt.Errorf("contact with email %s already exists", contact.Email)
	}

	if err := app.db.Model(contact).Create(); err != nil {
		return fmt.Errorf("failed to create contact: %v", err)
	}

	fmt.Printf("✅ Created contact: %s %s\n", contact.FirstName, contact.LastName)
	return nil
}

// List returns all contacts for the organization with pagination
func (app *ContactsApp) List(limit int, lastKey map[string]any) ([]Contact, map[string]any, error) {
	var contacts []Contact

	// Use prefix query on composite key
	query := app.db.Model(&Contact{}).
		Where("ID", "BEGINS_WITH", app.orgID+"#").
		Limit(limit)

	if lastKey != nil {
		// Resume from last evaluated key
		// In real implementation, this would use StartFrom
		// query = query.StartFrom(lastKey)
	}

	if err := query.All(&contacts); err != nil {
		return nil, nil, fmt.Errorf("failed to list contacts: %v", err)
	}

	// Extract org-specific data
	for i := range contacts {
		contacts[i].OrgID = app.orgID
	}

	// In real implementation, get LastEvaluatedKey from query result
	var nextKey map[string]any
	if len(contacts) == limit {
		nextKey = map[string]any{
			"ID": contacts[len(contacts)-1].ID,
		}
	}

	return contacts, nextKey, nil
}

// FindByEmail searches for a contact by email
func (app *ContactsApp) FindByEmail(email string) (*Contact, error) {
	var contacts []Contact

	// Use email GSI
	if err := app.db.Model(&Contact{}).
		Index("gsi-email").
		Where("Email", "=", email).
		All(&contacts); err != nil {
		return nil, fmt.Errorf("failed to search by email: %v", err)
	}

	// Filter by organization
	for _, contact := range contacts {
		if strings.HasPrefix(contact.ID, app.orgID+"#") {
			contact.OrgID = app.orgID
			return &contact, nil
		}
	}

	return nil, nil
}

// FindByPhone searches for contacts by phone number
func (app *ContactsApp) FindByPhone(phone string) ([]Contact, error) {
	var contacts []Contact

	// Normalize phone number (remove non-digits)
	normalizedPhone := normalizePhone(phone)

	// Use phone GSI
	if err := app.db.Model(&Contact{}).
		Index("gsi-phone").
		Where("Phone", "=", normalizedPhone).
		All(&contacts); err != nil {
		return nil, fmt.Errorf("failed to search by phone: %v", err)
	}

	// Filter by organization
	var orgContacts []Contact
	for _, contact := range contacts {
		if strings.HasPrefix(contact.ID, app.orgID+"#") {
			contact.OrgID = app.orgID
			orgContacts = append(orgContacts, contact)
		}
	}

	return orgContacts, nil
}

// SearchByName searches contacts by name pattern
func (app *ContactsApp) SearchByName(searchTerm string) ([]Contact, error) {
	// For demonstration, we'll get all contacts and filter
	// In production, use a search service like Elasticsearch
	contacts, _, err := app.List(1000, nil)
	if err != nil {
		return nil, err
	}

	searchLower := strings.ToLower(searchTerm)
	var matches []Contact

	for _, contact := range contacts {
		fullName := strings.ToLower(contact.FirstName + " " + contact.LastName)
		if strings.Contains(fullName, searchLower) ||
			strings.Contains(strings.ToLower(contact.Company), searchLower) {
			matches = append(matches, contact)
		}
	}

	return matches, nil
}

// GetFavorites returns all favorite contacts
func (app *ContactsApp) GetFavorites() ([]Contact, error) {
	contacts, _, err := app.List(1000, nil)
	if err != nil {
		return nil, err
	}

	var favorites []Contact
	for _, contact := range contacts {
		if contact.IsFavorite {
			favorites = append(favorites, contact)
		}
	}

	return favorites, nil
}

// Update modifies an existing contact
func (app *ContactsApp) Update(contactID string, updates map[string]any) error {
	// Build composite key
	id := fmt.Sprintf("%s#%s", app.orgID, contactID)

	// Get existing contact
	var contact Contact
	if err := app.db.Model(&Contact{}).Where("ID", "=", id).First(&contact); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("contact not found")
		}
		return fmt.Errorf("failed to get contact: %v", err)
	}

	// Apply updates
	if firstName, ok := updates["first_name"].(string); ok {
		contact.FirstName = firstName
		contact.FullName = fmt.Sprintf("%s#%s", contact.LastName, firstName)
	}

	if lastName, ok := updates["last_name"].(string); ok {
		contact.LastName = lastName
		contact.FullName = fmt.Sprintf("%s#%s", lastName, contact.FirstName)
	}

	if email, ok := updates["email"].(string); ok && email != contact.Email {
		// Check if new email is unique
		if existing, _ := app.FindByEmail(email); existing != nil {
			return fmt.Errorf("email %s is already in use", email)
		}
		contact.Email = email
	}

	if phone, ok := updates["phone"].(string); ok {
		contact.Phone = normalizePhone(phone)
	}

	if company, ok := updates["company"].(string); ok {
		contact.Company = company
	}

	if jobTitle, ok := updates["job_title"].(string); ok {
		contact.JobTitle = jobTitle
	}

	if favorite, ok := updates["favorite"].(bool); ok {
		contact.IsFavorite = favorite
	}

	if notes, ok := updates["notes"].(string); ok {
		contact.Notes = notes
	}

	if tags, ok := updates["tags"].([]string); ok {
		contact.Tags = tags
	}

	contact.UpdatedAt = time.Now()

	// Save changes
	if err := app.db.Model(&contact).Update(); err != nil {
		return fmt.Errorf("failed to update contact: %v", err)
	}

	fmt.Println("✅ Contact updated successfully")
	return nil
}

// Delete removes a contact
func (app *ContactsApp) Delete(contactID string) error {
	id := fmt.Sprintf("%s#%s", app.orgID, contactID)

	if err := app.db.Model(&Contact{}).Where("ID", "=", id).Delete(); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("contact not found")
		}
		return fmt.Errorf("failed to delete contact: %v", err)
	}

	fmt.Println("✅ Contact deleted successfully")
	return nil
}

// RecordInteraction updates the last contacted timestamp
func (app *ContactsApp) RecordInteraction(contactID string) error {
	now := time.Now()
	return app.Update(contactID, map[string]any{
		"last_contacted": &now,
	})
}

// BatchImport imports multiple contacts
func (app *ContactsApp) BatchImport(contacts []Contact) error {
	// Prepare contacts with composite keys
	for i := range contacts {
		contacts[i].ContactID = uuid.New().String()
		contacts[i].ID = fmt.Sprintf("%s#%s", app.orgID, contacts[i].ContactID)
		contacts[i].OrgID = app.orgID
		contacts[i].FullName = fmt.Sprintf("%s#%s", contacts[i].LastName, contacts[i].FirstName)
		contacts[i].CreatedAt = time.Now()
		contacts[i].UpdatedAt = time.Now()

		if contacts[i].Phone != "" {
			contacts[i].Phone = normalizePhone(contacts[i].Phone)
		}
	}

	// In real implementation, use BatchCreate
	// For now, create one by one
	successCount := 0
	for _, contact := range contacts {
		if err := app.db.Model(&contact).Create(); err != nil {
			fmt.Printf("Failed to import %s %s: %v\n", contact.FirstName, contact.LastName, err)
		} else {
			successCount++
		}
	}

	fmt.Printf("✅ Imported %d/%d contacts\n", successCount, len(contacts))
	return nil
}

// GetStats returns contact statistics
func (app *ContactsApp) GetStats() (map[string]any, error) {
	contacts, _, err := app.List(1000, nil)
	if err != nil {
		return nil, err
	}

	// Calculate statistics
	companies := make(map[string]int)
	tags := make(map[string]int)
	favoriteCount := 0
	withPhone := 0
	recentlyContacted := 0

	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	for _, contact := range contacts {
		if contact.Company != "" {
			companies[contact.Company]++
		}

		if contact.IsFavorite {
			favoriteCount++
		}

		if contact.Phone != "" {
			withPhone++
		}

		if contact.LastContactedAt != nil && contact.LastContactedAt.After(thirtyDaysAgo) {
			recentlyContacted++
		}

		for _, tag := range contact.Tags {
			tags[tag]++
		}
	}

	return map[string]any{
		"total_contacts":     len(contacts),
		"favorite_count":     favoriteCount,
		"with_phone":         withPhone,
		"recently_contacted": recentlyContacted,
		"companies":          companies,
		"tags":               tags,
	}, nil
}

// CLI functions

func (app *ContactsApp) printContacts(contacts []Contact, title string) {
	if len(contacts) == 0 {
		fmt.Printf("No contacts found%s.\n", title)
		return
	}

	fmt.Printf("\n👥 %s (%d contacts):\n", title, len(contacts))
	fmt.Println("─────────────────────────")

	for i, contact := range contacts {
		fav := ""
		if contact.IsFavorite {
			fav = "⭐ "
		}

		company := ""
		if contact.Company != "" {
			company = fmt.Sprintf(" at %s", contact.Company)
		}

		phone := ""
		if contact.Phone != "" {
			phone = fmt.Sprintf(" | 📱 %s", formatPhone(contact.Phone))
		}

		fmt.Printf("%d. %s%s %s%s\n", i+1, fav, contact.FirstName, contact.LastName, company)
		fmt.Printf("   📧 %s%s\n", contact.Email, phone)

		if len(contact.Tags) > 0 {
			fmt.Printf("   🏷️  %s\n", strings.Join(contact.Tags, ", "))
		}

		fmt.Printf("   ID: %s\n\n", contact.ContactID[:8])
	}
}

func (app *ContactsApp) runCLI() {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Printf("📇 Welcome to TableTheory Contacts App!\n")
	fmt.Printf("Organization: %s\n\n", app.orgID)

	// Show initial contacts
	if contacts, _, err := app.List(10, nil); err == nil {
		app.printContacts(contacts, "Recent Contacts")
	}

	fmt.Println("\nCommands:")
	fmt.Println("  add                  - Add a new contact")
	fmt.Println("  list [limit]        - List contacts")
	fmt.Println("  search <term>       - Search by name or company")
	fmt.Println("  email <email>       - Find by email")
	fmt.Println("  phone <number>      - Find by phone")
	fmt.Println("  favorites           - Show favorite contacts")
	fmt.Println("  favorite <num>      - Toggle favorite status")
	fmt.Println("  delete <num>        - Delete a contact")
	fmt.Println("  import              - Import sample contacts")
	fmt.Println("  stats               - Show statistics")
	fmt.Println("  quit                - Exit")
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
			if contacts, _, err := app.List(limit, nil); err == nil {
				app.printContacts(contacts, "Contacts")
			} else {
				fmt.Printf("Error: %v\n", err)
			}

		case "search":
			if len(parts) < 2 {
				fmt.Println("Usage: search <term>")
				continue
			}
			term := strings.Join(parts[1:], " ")
			if contacts, err := app.SearchByName(term); err == nil {
				app.printContacts(contacts, fmt.Sprintf("Search results for '%s'", term))
			} else {
				fmt.Printf("Error: %v\n", err)
			}

		case "email":
			if len(parts) < 2 {
				fmt.Println("Usage: email <email>")
				continue
			}
			if contact, err := app.FindByEmail(parts[1]); err == nil {
				if contact != nil {
					app.printContacts([]Contact{*contact}, "Contact found")
				} else {
					fmt.Println("No contact found with that email")
				}
			} else {
				fmt.Printf("Error: %v\n", err)
			}

		case "phone":
			if len(parts) < 2 {
				fmt.Println("Usage: phone <number>")
				continue
			}
			if contacts, err := app.FindByPhone(parts[1]); err == nil {
				app.printContacts(contacts, fmt.Sprintf("Contacts with phone %s", parts[1]))
			} else {
				fmt.Printf("Error: %v\n", err)
			}

		case "favorites":
			if contacts, err := app.GetFavorites(); err == nil {
				app.printContacts(contacts, "Favorite Contacts")
			} else {
				fmt.Printf("Error: %v\n", err)
			}

		case "favorite":
			if len(parts) < 2 {
				fmt.Println("Usage: favorite <number>")
				continue
			}
			app.handleFavorite(parts[1])

		case "delete":
			if len(parts) < 2 {
				fmt.Println("Usage: delete <number>")
				continue
			}
			app.handleDelete(parts[1])

		case "import":
			app.handleImport()

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
			fmt.Println("Unknown command. Try: add, list, search, email, phone, favorites, delete, import, stats, quit")
		}
	}
}

func (app *ContactsApp) handleAdd(scanner *bufio.Scanner) {
	contact := &Contact{}

	fmt.Print("First Name: ")
	scanner.Scan()
	contact.FirstName = strings.TrimSpace(scanner.Text())

	fmt.Print("Last Name: ")
	scanner.Scan()
	contact.LastName = strings.TrimSpace(scanner.Text())

	fmt.Print("Email: ")
	scanner.Scan()
	contact.Email = strings.TrimSpace(scanner.Text())

	fmt.Print("Phone: ")
	scanner.Scan()
	contact.Phone = strings.TrimSpace(scanner.Text())

	fmt.Print("Company: ")
	scanner.Scan()
	contact.Company = strings.TrimSpace(scanner.Text())

	fmt.Print("Job Title: ")
	scanner.Scan()
	contact.JobTitle = strings.TrimSpace(scanner.Text())

	fmt.Print("Tags (comma-separated): ")
	scanner.Scan()
	tagInput := strings.TrimSpace(scanner.Text())

	if tagInput != "" {
		for _, tag := range strings.Split(tagInput, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				contact.Tags = append(contact.Tags, tag)
			}
		}
	}

	if err := app.Create(contact); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func (app *ContactsApp) handleFavorite(numStr string) {
	num, err := strconv.Atoi(numStr)
	if err != nil {
		fmt.Println("Invalid number")
		return
	}

	contacts, _, err := app.List(100, nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if num < 1 || num > len(contacts) {
		fmt.Println("Invalid contact number")
		return
	}

	contact := contacts[num-1]
	err = app.Update(contact.ContactID, map[string]any{
		"favorite": !contact.IsFavorite,
	})

	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func (app *ContactsApp) handleDelete(numStr string) {
	num, err := strconv.Atoi(numStr)
	if err != nil {
		fmt.Println("Invalid number")
		return
	}

	contacts, _, err := app.List(100, nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if num < 1 || num > len(contacts) {
		fmt.Println("Invalid contact number")
		return
	}

	if err := app.Delete(contacts[num-1].ContactID); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func (app *ContactsApp) handleImport() {
	// Sample contacts for import
	sampleContacts := []Contact{
		{
			FirstName: "Alice",
			LastName:  "Johnson",
			Email:     "alice@example.com",
			Phone:     "555-0101",
			Company:   "Tech Corp",
			JobTitle:  "Software Engineer",
			Tags:      []string{"engineering", "client"},
		},
		{
			FirstName: "Bob",
			LastName:  "Smith",
			Email:     "bob@example.com",
			Phone:     "555-0102",
			Company:   "Design Studio",
			JobTitle:  "Creative Director",
			Tags:      []string{"design", "vendor"},
		},
		{
			FirstName: "Carol",
			LastName:  "Williams",
			Email:     "carol@example.com",
			Phone:     "555-0103",
			Company:   "Marketing Inc",
			JobTitle:  "Marketing Manager",
			Tags:      []string{"marketing", "partner"},
		},
	}

	if err := app.BatchImport(sampleContacts); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func (app *ContactsApp) printStats(stats map[string]any) {
	fmt.Println("\n📊 Contact Statistics:")
	fmt.Println("─────────────────────")
	fmt.Printf("Total Contacts: %v\n", stats["total_contacts"])
	fmt.Printf("Favorites: %v\n", stats["favorite_count"])
	fmt.Printf("With Phone: %v\n", stats["with_phone"])
	fmt.Printf("Recently Contacted: %v\n", stats["recently_contacted"])

	if companies, ok := stats["companies"].(map[string]int); ok && len(companies) > 0 {
		fmt.Println("\nTop Companies:")
		for company, count := range companies {
			fmt.Printf("  - %s: %d contacts\n", company, count)
		}
	}

	if tags, ok := stats["tags"].(map[string]int); ok && len(tags) > 0 {
		fmt.Println("\nPopular Tags:")
		for tag, count := range tags {
			fmt.Printf("  - %s (%d)\n", tag, count)
		}
	}
	fmt.Println()
}

// Utility functions

func normalizePhone(phone string) string {
	// Remove all non-digits
	var normalized strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

func formatPhone(phone string) string {
	if len(phone) == 10 {
		return fmt.Sprintf("(%s) %s-%s", phone[:3], phone[3:6], phone[6:])
	}
	return phone
}

func main() {
	// Get organization ID from environment or use default
	orgID := os.Getenv("ORG_ID")
	if orgID == "" {
		orgID = "org-" + uuid.New().String()[:8]
	}

	app, err := NewContactsApp(orgID)
	if err != nil {
		log.Fatalf("Failed to initialize app: %v", err)
	}

	app.runCLI()
}
