# Cursor-Based Pagination in Blog Example

This document explains the cursor-based pagination implementation in the blog example.

## Overview

The blog example uses cursor-based pagination for efficient navigation through large result sets. This approach is more stable than offset-based pagination when dealing with frequently changing data.

## How It Works

### Cursor Format

The cursor is a base64-encoded JSON object containing:
- `p` (LastPublishedAt): The timestamp of the last item in the current page
- `i` (LastID): The ID of the last item (for tie-breaking)
- `d` (Direction): Currently always "next" (for future bidirectional support)

### API Usage

#### Request
```http
GET /posts?status=published&limit=20&cursor=eyJwIjoiMjAyNC0wMS0xNVQxMDozMDowMFoiLCJpIjoicG9zdC0xMjMifQ==
```

Query parameters:
- `limit`: Number of items per page (default: 20, max: 100)
- `cursor`: Opaque cursor string from previous response
- `status`: Filter by post status (draft, published, archived)
- `author_id`: Filter by author
- `category_id`: Filter by category

#### Response
```json
{
  "posts": [...],
  "next_cursor": "eyJwIjoiMjAyNC0wMS0xNVQwOTowMDowMFoiLCJpIjoicG9zdC00NTYifQ==",
  "has_more": true,
  "limit": 20
}
```

Response fields:
- `posts`: Array of post objects
- `next_cursor`: Cursor for the next page (empty if no more results)
- `has_more`: Boolean indicating if more results exist
- `limit`: The limit used for this request

## Implementation Details

### Cursor Encoding/Decoding

```go
// Encode cursor from last post
cursor := EncodeCursor(lastPost.PublishedAt, lastPost.ID)

// Decode cursor for next request
cursorData, err := DecodeCursor(request.QueryStringParameters["cursor"])
```

### Query Building

The implementation handles different indexes:
1. **Status-based queries**: Uses `gsi-status-date` index with PublishedAt as sort key
2. **Author-based queries**: Uses `gsi-author` index with CreatedAt as sort key

### Edge Cases Handled

1. **Empty Results**: Returns empty cursor and `has_more: false`
2. **Invalid Cursor**: Returns 400 Bad Request
3. **Last Page**: Determined by fetching limit+1 items
4. **Tie Breaking**: Uses post ID when timestamps are identical

## Example Client Implementation

```javascript
async function fetchAllPosts() {
  let cursor = '';
  let allPosts = [];
  
  do {
    const response = await fetch(`/posts?cursor=${cursor}&limit=50`);
    const data = await response.json();
    
    allPosts = allPosts.concat(data.posts);
    cursor = data.next_cursor;
    
  } while (cursor);
  
  return allPosts;
}
```

## Benefits

1. **Stable Pagination**: New posts don't affect existing cursors
2. **Efficient**: Uses DynamoDB indexes effectively
3. **Scalable**: No offset calculations needed
4. **Stateless**: Cursor contains all needed information

## Limitations

1. **Forward-Only**: Currently only supports forward pagination
2. **No Random Access**: Can't jump to specific pages
3. **Cursor Size**: Grows with field sizes (minimal impact)

## Future Enhancements

1. **Bidirectional Pagination**: Add previous cursor support
2. **Cursor Compression**: Use more efficient encoding
3. **Multi-Index Support**: Handle complex query combinations
4. **Cursor Expiration**: Add TTL for security 