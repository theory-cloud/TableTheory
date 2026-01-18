# Contacts App - Advanced TableTheory Patterns

The most advanced basic example, demonstrating composite keys, complex filtering, batch operations, and real-world patterns.

## What You'll Learn

- Composite primary keys for multi-tenancy
- Multiple Global Secondary Indexes
- Advanced search patterns
- Batch operations
- Data normalization techniques
- Pagination with cursors
- Complex data structures

## Key Features

- **Multi-Organization**: Contacts isolated by organization
- **Multiple Search Methods**: Email, phone, name, tags
- **Favorites System**: Quick access to important contacts
- **Batch Import**: Import multiple contacts at once
- **Rich Data Model**: Addresses, custom fields, metadata
- **Statistics**: Analytics and insights

## Quick Start

### 1. Start DynamoDB Local

```bash
docker-compose up -d
```

### 2. Run the Application

```bash
go mod tidy
go run main.go
```

### 3. Example Session

```
📇 Welcome to TableTheory Contacts App!
Organization: org-a1b2c3d4

> add
First Name: John
Last Name: Doe
Email: john.doe@example.com
Phone: 555-123-4567
Company: Acme Corp
Job Title: CEO
Tags (comma-separated): vip, client, decision-maker
✅ Created contact: John Doe

> import
✅ Imported 3/3 contacts

> list
👥 Contacts (4 contacts):
─────────────────────────
1. John Doe at Acme Corp
   📧 john.doe@example.com | 📱 (555) 123-4567
   🏷️  vip, client, decision-maker
   ID: a1b2c3d4

2. Alice Johnson at Tech Corp
   📧 alice@example.com | 📱 (555) 010-1000
   🏷️  engineering, client
   ID: e5f6g7h8

> search tech
👥 Search results for 'tech' (1 contacts):
─────────────────────────
1. Alice Johnson at Tech Corp
   📧 alice@example.com | 📱 (555) 010-1000
   🏷️  engineering, client
   ID: e5f6g7h8

> email john.doe@example.com
👥 Contact found (1 contacts):
─────────────────────────
1. John Doe at Acme Corp
   📧 john.doe@example.com | 📱 (555) 123-4567
   🏷️  vip, client, decision-maker
   ID: a1b2c3d4
```

## Model Design Deep Dive

### Composite Primary Key

```go
// Primary key format: OrgID#ContactID
ID string `theorydb:"pk"`

// Example: "org-123#contact-456"
// This allows efficient queries for all contacts in an org
```

**Benefits:**
- Natural data isolation per organization
- Efficient org-level queries
- No cross-org data leaks
- Supports multi-tenancy

### Index Strategy

1. **Primary Table**
   - PK: `ID` (OrgID#ContactID)
   - Use: Direct lookups, org listings

2. **GSI: gsi-email**
   - PK: `Email`
   - Use: Find contact by email (unique check)

3. **GSI: gsi-phone**
   - PK: `Phone` (normalized)
   - Use: Find contacts by phone number

4. **GSI: gsi-name**
   - PK: `FullName` (LastName#FirstName)
   - Use: Alphabetical listings (future feature)

### Complex Data Structures

```go
// Nested struct for addresses
Address struct {
    Street  string
    City    string
    State   string
    ZipCode string
    Country string
}

// Map for extensible custom fields
CustomFields map[string]string

// Set for tags
Tags []string `theorydb:"set"`

// Optional timestamp
LastContactedAt *time.Time
```

## Advanced Patterns

### 1. Composite Key Queries

```go
// Get all contacts for an organization
query.Where("ID", "BEGINS_WITH", orgID+"#")

// This efficiently retrieves only the org's data
```

### 2. Email Uniqueness

```go
// Check email uniqueness within org
existing, _ := app.FindByEmail(email)
if existing != nil {
    return fmt.Errorf("email already exists")
}
```

### 3. Phone Normalization

```go
// Store phones in consistent format
func normalizePhone(phone string) string {
    // Remove all non-digits
    // "555-123-4567" -> "5551234567"
}
```

### 4. Batch Operations

```go
// Prepare batch with composite keys
for i := range contacts {
    contacts[i].ID = fmt.Sprintf("%s#%s", orgID, uuid.New())
    contacts[i].CreatedAt = time.Now()
}

// In production, use DynamoDB batch operations
// Max 25 items per batch write
```

### 5. Search Strategies

```go
// Email search (exact match via GSI)
Index("gsi-email").Where("Email", "=", email)

// Name search (prefix match possible with GSI)
Index("gsi-name").Where("FullName", "BEGINS_WITH", "Smith#")

// Tag search (requires scan + filter)
// Consider dedicated tag index for heavy use
```

## Performance Optimization

### Query Efficiency

| Operation | Method | Performance |
|-----------|--------|-------------|
| List org contacts | Query with prefix | ⚡ Fast |
| Find by email | Query on GSI | ⚡ Fast |
| Find by phone | Query on GSI | ⚡ Fast |
| Search by name | Scan + filter | 🐌 Slow |
| Search by tag | Scan + filter | 🐌 Slow |

### Optimization Strategies

1. **Pagination**
   ```go
   // Use LastEvaluatedKey for cursor
   query.StartFrom(lastKey).Limit(25)
   ```

2. **Projections**
   ```go
   // Only fetch needed fields
   query.Select("ID", "FirstName", "LastName", "Email")
   ```

3. **Caching**
   - Cache frequently accessed contacts
   - Cache search results
   - Invalidate on updates

4. **Search Service**
   - For full-text search, use Elasticsearch
   - Stream changes via DynamoDB Streams
   - Keep search index in sync

## Multi-Tenancy Considerations

### Data Isolation

```go
// Always include OrgID in queries
id := fmt.Sprintf("%s#%s", orgID, contactID)

// Verify ownership on updates
if !strings.HasPrefix(contact.ID, orgID+"#") {
    return errors.New("unauthorized")
}
```

### Scaling Strategies

1. **Table per tenant** (for large tenants)
2. **Shared table** with composite keys (shown here)
3. **Hybrid approach** based on tenant size

### Security Best Practices

- Never expose internal IDs
- Always validate org ownership
- Use IAM policies for additional security
- Audit all operations

## Exercises

1. **Add Groups**: Implement contact groups/lists
2. **Import CSV**: Add CSV import functionality
3. **Export**: Implement data export (JSON/CSV)
4. **Merge Duplicates**: Detect and merge duplicate contacts
5. **Activity Feed**: Track all contact interactions
6. **Smart Search**: Implement fuzzy matching
7. **Bulk Operations**: Add bulk update/delete

## Production Considerations

### Error Handling

```go
// Distinguish between not found and errors
if errors.IsNotFound(err) {
    // Handle missing item
} else if err != nil {
    // Handle system error
}
```

### Monitoring

- Track query latencies
- Monitor hot partitions
- Alert on error rates
- Track storage growth

### Backup Strategy

- Enable point-in-time recovery
- Regular backups to S3
- Test restore procedures
- Document recovery process

## Troubleshooting

### "Hot partition" warnings
- Distribute load across partition keys
- Consider sharding large organizations

### Slow searches
- Add appropriate indexes
- Implement caching layer
- Consider search service

### Duplicate emails
- Implement proper uniqueness checks
- Consider using email as partition key

## Next Steps

You've now mastered:
- ✅ Basic CRUD (Todo app)
- ✅ Indexes and queries (Notes app)
- ✅ Complex patterns (Contacts app)

Ready for:
- 🚀 **Blog Example**: Content management
- 🚀 **Blog Platform**: Rich content relationships
- 🚀 **Payment**: Financial data patterns
- 🚀 **Multi-tenant**: SaaS architectures

## Key Takeaways

✅ **Composite keys enable multi-tenancy**
✅ **Multiple indexes support different access patterns**
✅ **Normalization improves query efficiency**
✅ **Batch operations reduce API calls**
✅ **Search requires careful design**
✅ **Always consider data isolation** 
