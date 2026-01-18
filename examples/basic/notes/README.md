# Notes App - Indexes and Complex Queries

Building on the Todo app, this example introduces more advanced TableTheory features including indexes, sets, and complex queries.

## What You'll Learn

- Global Secondary Indexes (GSI) for efficient queries
- Working with sets for tags
- Composite sort keys for time-based queries
- Query operations vs Scan
- Data modeling for multi-user systems
- Statistics and aggregations

## Key Features

- **Multi-user support**: Each user has their own notes
- **Categories**: Organize notes by category
- **Tags**: Flexible tagging with sets
- **Search**: Find notes by tag, category, or date
- **Statistics**: Track word counts and usage

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
🗒️  Welcome to TableTheory Notes App!
Logged in as: demo-user

> add
Title: TableTheory Study Notes
Category (personal/work/ideas/other): work
Content: Learning about indexes and query patterns in DynamoDB
Tags (comma-separated): dynamodb, learning, database
✅ Created note: TableTheory Study Notes

> add
Title: Project Ideas
Category (personal/work/ideas/other): ideas
Content: Build a serverless blog platform using TableTheory
Tags (comma-separated): project, serverless, blog
✅ Created note: Project Ideas

> list
📝 Your Notes (2 notes):
───────────────────────
1. TableTheory Study Notes (work) [dynamodb, learning, database]
   Learning about indexes and query patterns in DynamoDB
   📅 2024-01-15 | 📝 9 words | ID: abc12345

2. Project Ideas (ideas) [project, serverless, blog]
   Build a serverless blog platform using TableTheory
   📅 2024-01-15 | 📝 8 words | ID: def67890

> category work
📝 Notes in 'work' (1 notes):
───────────────────────
1. TableTheory Study Notes (work) [dynamodb, learning, database]
   Learning about indexes and query patterns in DynamoDB
   📅 2024-01-15 | 📝 9 words | ID: abc12345

> tag learning
📝 Notes tagged 'learning' (1 notes):
───────────────────────
1. TableTheory Study Notes (work) [dynamodb, learning, database]
   Learning about indexes and query patterns in DynamoDB
   📅 2024-01-15 | 📝 9 words | ID: abc12345
```

## Model Design

### Primary Key Structure

```go
type Note struct {
    ID     string `theorydb:"pk"`                    // Partition key
    UserID string `theorydb:"index:gsi-user,pk"`     // GSI partition key
    // ... other fields
}
```

### Indexes Explained

1. **Primary Table**
   - Partition Key: `ID`
   - Use: Direct lookups by note ID

2. **GSI: gsi-user**
   - Partition Key: `UserID`
   - Sort Key: `CreatedAt`
   - Use: Get all notes for a user, sorted by time

3. **GSI: gsi-category**
   - Partition Key: `Category`
   - Use: Get all notes in a category

### Working with Sets

```go
// Tags are stored as a DynamoDB set
Tags []string `theorydb:"set"`

// Create note with tags
note := &Note{
    Tags: []string{"important", "project", "todo"},
}

// Sets guarantee unique values
// Order is not preserved
```

## Query Patterns

### 1. Query by User (Most Efficient)

```go
// Uses GSI to query efficiently
notes, err := db.Model(&Note{}).
    Index("gsi-user").
    Where("UserID", "=", userID).
    Limit(10).
    All(&notes)
```

### 2. Query by Category

```go
// Uses category GSI
notes, err := db.Model(&Note{}).
    Index("gsi-category").
    Where("Category", "=", "work").
    All(&notes)
```

### 3. Search by Tag (Less Efficient)

```go
// Requires scanning and filtering
// Consider a tag GSI for heavy tag usage
var filtered []Note
for _, note := range allNotes {
    if contains(note.Tags, targetTag) {
        filtered = append(filtered, note)
    }
}
```

### 4. Time-Based Queries

```go
// If CreatedAt was a sort key, we could do:
// Where("UserID", "=", userID).
// Where("CreatedAt", ">", timestamp)

// Without sort key, we filter in memory
cutoff := time.Now().AddDate(0, 0, -7)
var recent []Note
for _, note := range userNotes {
    if note.CreatedAt.After(cutoff) {
        recent = append(recent, note)
    }
}
```

## Best Practices Demonstrated

### 1. Index Design

- **Access patterns first**: Design indexes based on how you query
- **Minimize indexes**: Each index costs storage and writes
- **Composite keys**: Use sort keys for range queries

### 2. Multi-User Isolation

```go
// Always filter by user to ensure data isolation
query.Where("UserID", "=", currentUser)

// Verify ownership before updates/deletes
if note.UserID != currentUser {
    return errors.New("unauthorized")
}
```

### 3. Efficient Querying

```go
// Good: Use index
db.Model(&Note{}).Index("gsi-user").Where("UserID", "=", id)

// Bad: Scan entire table
db.Model(&Note{}).Scan(&notes) // Then filter

// Better: Add appropriate index
```

### 4. Working with Sets

```go
// DynamoDB sets are great for:
// - Tags
// - Categories
// - Unique lists

// But remember:
// - Can't query directly on set contents
// - Order is not preserved
// - Updates replace the entire set
```

## Exercises

1. **Add Folders**: Implement hierarchical folders for notes
2. **Full-Text Search**: Add content search (consider DynamoDB Streams + Elasticsearch)
3. **Sharing**: Allow sharing notes between users
4. **Versions**: Track note history with versions
5. **Sort Options**: Add different sort orders (alphabetical, word count, etc.)

## Performance Considerations

### Query vs Scan

| Operation | Use When | Cost |
|-----------|----------|------|
| Query | You know the partition key | Efficient, only reads matching items |
| Scan | Need to check every item | Expensive, reads entire table |

### Index Costs

- Each GSI is essentially a copy of your data
- Updates write to main table + all indexes
- Choose indexes carefully based on access patterns

### Optimization Tips

1. **Batch operations**: Use batch get/write for multiple items
2. **Projections**: Only include needed attributes in indexes
3. **Pagination**: Always paginate large result sets
4. **Caching**: Consider caching frequently accessed data

## Troubleshooting

### "Index not found" error
Make sure the table was created with indexes. Delete and recreate if needed.

### Slow queries
Check if you're using Scan instead of Query. Add appropriate indexes.

### Tags not working
Ensure you're using the `set` tag in your model definition.

## Next Steps

After mastering indexes and queries, move on to:
- **Contacts App**: Learn composite keys and advanced filtering
- **Blog Example**: See complex relationships and indexing strategies
- **Blog Example**: Understand content management patterns

## Key Takeaways

✅ **Indexes are crucial**: Design them based on access patterns
✅ **Query > Scan**: Always prefer indexed queries
✅ **Model for queries**: Structure data to support your queries
✅ **Sets for uniqueness**: Use sets for tags and categories
✅ **Filter in DB**: Push filtering to DynamoDB when possible 
