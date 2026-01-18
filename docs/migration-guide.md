# TableTheory Migration Guide

This guide assists in migrating existing Go applications to use TableTheory, focusing on transitions from raw AWS SDK calls or other ORMs.

Multi-language migration guides:

- TypeScript: [ts/docs/migration-guide.md](../ts/docs/migration-guide.md)
- Python: [py/docs/migration-guide.md](../py/docs/migration-guide.md)

## From Raw AWS SDK for Go (v2)

**Problem:** Directly using the AWS SDK for Go v2 for DynamoDB operations often leads to verbose code, manual attribute marshaling, and lacks type safety. It also requires explicit context management for every call.

**Solution:** Replace direct SDK calls with TableTheory's fluent, type-safe API. TableTheory handles marshaling/unmarshaling, context propagation, and error handling automatically.

### Example: Creating an Item

```go
// ❌ OLD WAY: Raw AWS SDK v2
package main

import (
    "context"
    "log"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
    "github.com/aws/aws-sdk-go-v2/config"
)

type User struct {
    ID    string
    Email string
    Name  string
}

func createUserSDK(ctx context.Context, user User) error {
    cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
    if err != nil {
        return err
    }
    svc := dynamodb.NewFromConfig(cfg)

    item := map[string]types.AttributeValue{
        "ID":    &types.AttributeValueMemberS{Value: user.ID},
        "Email": &types.AttributeValueMemberS{Value: user.Email},
        "Name":  &types.AttributeValueMemberS{Value: user.Name},
    }

    _, err = svc.PutItem(ctx, &dynamodb.PutItemInput{
        TableName: aws.String("users"),
        Item:      item,
    })
    return err
}

func main() {
    user := User{ID: "sdk_user_1", Email: "sdk@example.com", Name: "SDK User"}
    if err := createUserSDK(context.TODO(), user); err != nil {
        log.Fatalf("Failed to create user with SDK: %v", err)
    }
    log.Println("SDK user created.")
}
```

```go
// ✅ NEW WAY: TableTheory
package main

import (
    "context"
    "log"

    "github.com/theory-cloud/tabletheory"
    "github.com/theory-cloud/tabletheory/pkg/session"
)

type User struct {
    ID    string `theorydb:"pk" json:"id"`
    Email string `theorydb:"sk" json:"email"`
    Name  string `json:"name"`
}

func createUserTableTheory(ctx context.Context, db theorydb.DB, user *User) error {
    return db.WithContext(ctx).Model(user).Create()
}

func main() {
    db, err := theorydb.New(session.Config{Region: "us-east-1"})
    if err != nil {
        log.Fatalf("Failed to initialize TableTheory: %v", err)
    }

    user := &User{ID: "orm_user_1", Email: "orm@example.com", Name: "TableTheory User"}
    if err := createUserTableTheory(context.TODO(), db, user); err != nil {
        log.Fatalf("Failed to create user with TableTheory: %v", err)
    }
    log.Println("TableTheory user created.")
}
```

### Benefits of Migrating to TableTheory

- **Reduced Boilerplate**: Significantly less code required for common CRUD operations.
- **Type Safety**: Compile-time checks prevent common runtime errors related to attribute names and types.
- **Automatic Marshaling**: Handles conversion between Go structs and DynamoDB `AttributeValue` maps.
- **Lambda Optimization**: Built-in features for cold-start reduction and connection reuse in serverless environments.
- **Fluent API**: Chainable methods make queries and transactions more readable and maintainable.

## From Other ORMs (e.g., GORM for SQL)

**Problem:** SQL ORMs are designed for relational databases and do not translate well to DynamoDB's NoSQL, key-value, and document-oriented model. Concepts like joins and complex secondary indexes are fundamentally different.

**Solution:** Adapt your data models and query patterns to be DynamoDB-native. TableTheory provides an ORM-like experience while respecting DynamoDB's strengths.

### Key Differences and Adaptations

1.  **Data Modeling**: Think about Partition Keys (PK) and Sort Keys (SK) for efficient access patterns, not just primary keys.
    - **SQL**: `id INT PRIMARY KEY`, `name VARCHAR(255)`
    - **DynamoDB (TableTheory)**: `ID string `theorydb:"pk"`, `SK string `theorydb:"sk"`

2.  **Joins**: DynamoDB does not support joins. Denormalize data or use multiple `BatchGet` calls.

3.  **Querying**: Prioritize queries by PK/SK. Use Global Secondary Indexes (GSIs) for alternate access patterns.

```go
// ❌ OLD WAY: GORM (SQL-like)
func getActiveUsersGORM(db *gorm.DB) ([]User, error) {
    var users []User
    result := db.Where("status = ?", "active").Find(&users)
    return users, result.Error
}

// ✅ NEW WAY: TableTheory (DynamoDB-native)
// Assumes a GSI named "status-index" with 'Status' as its PK
func getActiveUsersTableTheory(db theorydb.DB) ([]User, error) {
    var users []User
    err := db.Model(&User{}).
        Index("status-index").        // Explicitly use the GSI
        Where("Status", "=", "active").
        All(&users)
    return users, err
}
```
