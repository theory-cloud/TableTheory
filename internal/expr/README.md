# Expression Builder

The `expr` package provides a builder for constructing DynamoDB expressions with proper handling of reserved words, placeholders, and complex operations.

## Key Features

- Automatic placeholder generation for attribute names and values
- Reserved word detection and escaping
- Support for all DynamoDB expression types
- Function-based update operations (list_append, if_not_exists)

## Available Methods

### Query Operations
- `AddKeyCondition(field, operator, value)` - Add key conditions for Query operations
- `AddFilterCondition(logicalOp, field, operator, value)` - Add filter conditions with AND/OR logic
- `AddGroupFilter(logicalOp, components)` - Add grouped filter expressions
- `AddProjection(fields...)` - Add projection expressions

### Update Operations
- `AddUpdateSet(field, value)` - SET field = value
- `AddUpdateAdd(field, value)` - ADD field value (atomic counters)
- `AddUpdateRemove(field)` - REMOVE field
- `AddUpdateDelete(field, value)` - DELETE value FROM field (for sets)
- `AddUpdateFunction(field, function, args...)` - Function-based updates

### Supported Functions

#### list_append
Appends or prepends values to a list:
```go
// Append to end: SET field = list_append(field, :value)
builder.AddUpdateFunction("Tags", "list_append", "Tags", []string{"new"})

// Prepend to beginning: SET field = list_append(:value, field)  
builder.AddUpdateFunction("Tags", "list_append", []string{"new"}, "Tags")
```

#### if_not_exists
Sets a field only if it doesn't exist:
```go
// SET field = if_not_exists(field, :default)
builder.AddUpdateFunction("Description", "if_not_exists", "Description", "Default text")
```

### Condition Operations
- `AddConditionExpression(field, operator, value)` - Add conditions for conditional updates

## Supported Operators

- Comparison: `=`, `!=`, `<>`, `<`, `<=`, `>`, `>=`
- Range: `BETWEEN`
- Membership: `IN`
- String: `BEGINS_WITH`, `CONTAINS`
- Existence: `EXISTS`, `NOT_EXISTS`

## Example Usage

```go
builder := expr.NewBuilder()

// Add key condition
builder.AddKeyCondition("UserID", "=", "user123")

// Add filter
builder.AddFilterCondition("AND", "Status", "=", "active")

// Add update with list append
builder.AddUpdateSet("Name", "New Name")
builder.AddUpdateFunction("Tags", "list_append", "Tags", []string{"updated"})

// Build the expressions
components := builder.Build()

// Use components in DynamoDB operations
// components.KeyConditionExpression
// components.FilterExpression  
// components.UpdateExpression
// components.ExpressionAttributeNames
// components.ExpressionAttributeValues
```

## Reserved Words

The builder automatically detects and escapes DynamoDB reserved words by prefixing them with `#`. All 600+ reserved words are handled, including common ones like:
- STATUS, STATE, NAME, TYPE
- USER, GROUP, ROLE
- DATA, VALUE, KEY
- And many more...

## Thread Safety

The Builder is not thread-safe. Create a new instance for each operation or use proper synchronization. 