package handlers

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/examples/blog/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
)

// PostHandler handles blog post operations
type PostHandler struct {
	db core.ExtendedDB
}

// NewPostHandler creates a new post handler
func NewPostHandler() (*PostHandler, error) {
	db, err := tabletheory.New(tabletheory.Config{
		Region: "us-east-1",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamoDB: %w", err)
	}

	// Register models
	db.Model(&models.Post{})
	db.Model(&models.Author{})
	db.Model(&models.Category{})
	db.Model(&models.Tag{})
	db.Model(&models.SearchIndex{})
	db.Model(&models.Analytics{})

	return &PostHandler{db: db}, nil
}

// HandleRequest routes requests to appropriate handlers
func (h *PostHandler) HandleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	switch request.HTTPMethod {
	case "GET":
		if request.PathParameters["slug"] != "" {
			return h.getPostBySlug(ctx, request)
		}
		return h.listPosts(ctx, request)
	case "POST":
		return h.createPost(ctx, request)
	case "PUT":
		return h.updatePost(ctx, request)
	case "DELETE":
		return h.deletePost(ctx, request)
	default:
		return errorResponse(http.StatusMethodNotAllowed, "Method not allowed"), nil
	}
}

// listPosts returns paginated list of posts
func (h *PostHandler) listPosts(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Parse query parameters
	status := request.QueryStringParameters["status"]
	if status == "" {
		status = models.PostStatusPublished
	}

	limit, _ := strconv.Atoi(request.QueryStringParameters["limit"])
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Cursor-based pagination
	cursor := request.QueryStringParameters["cursor"]
	authorID := request.QueryStringParameters["author_id"]
	categoryID := request.QueryStringParameters["category_id"]

	// Decode cursor if provided
	cursorData, err := DecodeCursor(cursor)
	if err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid cursor"), nil
	}

	// Build query
	query := h.db.Model(&models.Post{}).
		Index("gsi-status-date").
		Where("Status", "=", status).
		OrderBy("PublishedAt", "DESC").
		Limit(limit + 1) // Request one extra to check if there are more results

	// Apply cursor if provided
	if cursorData != nil {
		query = query.Where("PublishedAt", "<=", cursorData.LastPublishedAt)
	}

	// Apply filters
	if authorID != "" {
		// Use author index instead
		query = h.db.Model(&models.Post{}).
			Index("gsi-author").
			Where("AuthorID", "=", authorID).
			Filter("Status", "=", status).
			OrderBy("CreatedAt", "DESC").
			Limit(limit + 1)

		// Apply cursor for author index
		if cursorData != nil {
			query = query.Where("CreatedAt", "<=", cursorData.LastPublishedAt)
		}
	} else if categoryID != "" {
		query = query.Filter("CategoryID", "=", categoryID)
	}

	// Tag filtering using DynamoDB expressions would require a contains condition.
	// This can be added in the future when tag queries are supported.

	// Execute query
	var posts []*models.Post
	err = query.All(&posts)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch posts"), nil
	}

	// Check if there are more results
	hasMore := len(posts) > limit
	if hasMore {
		// Remove the extra post we fetched
		posts = posts[:limit]
	}

	// Generate next cursor
	var nextCursor string
	if hasMore && len(posts) > 0 {
		lastPost := posts[len(posts)-1]
		if authorID != "" {
			// For author queries, use CreatedAt
			nextCursor = EncodeCursor(lastPost.CreatedAt, lastPost.ID)
		} else {
			// For status queries, use PublishedAt
			nextCursor = EncodeCursor(lastPost.PublishedAt, lastPost.ID)
		}
	}

	// Enrich posts with author info (in production, consider caching)
	authorIDs := make([]string, 0, len(posts))
	authorMap := make(map[string]*models.Author)

	for _, post := range posts {
		authorIDs = append(authorIDs, post.AuthorID)
	}

	// Batch get authors
	if len(authorIDs) > 0 {
		var authors []*models.Author
		if err := h.db.Model(&models.Author{}).
			Where("ID", "in", authorIDs).
			All(&authors); err != nil {
			return errorResponse(http.StatusInternalServerError, "Failed to fetch authors"), nil
		}

		for _, author := range authors {
			authorMap[author.ID] = author
		}
	}

	// Build response
	type enrichedPost struct {
		*models.Post
		Author *models.Author `json:"author,omitempty"`
	}

	enrichedPosts := make([]enrichedPost, len(posts))
	for i, post := range posts {
		enrichedPosts[i] = enrichedPost{
			Post:   post,
			Author: authorMap[post.AuthorID],
		}
	}

	response := map[string]any{
		"posts":       enrichedPosts,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
		"limit":       limit,
	}

	return successResponse(http.StatusOK, response), nil
}

// getPostBySlug retrieves a post by its slug
func (h *PostHandler) getPostBySlug(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	slug := request.PathParameters["slug"]
	if slug == "" {
		return errorResponse(http.StatusBadRequest, "Slug is required"), nil
	}

	// Get post by slug
	var post models.Post
	err := h.db.Model(&models.Post{}).
		Index("gsi-slug").
		Where("Slug", "=", slug).
		First(&post)

	if err != nil {
		if stdErrors.Is(err, errors.ErrItemNotFound) {
			return errorResponse(http.StatusNotFound, "Post not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch post"), nil
	}

	// Only return published posts to public
	if post.Status != models.PostStatusPublished && !isAuthorized(request) {
		return errorResponse(http.StatusNotFound, "Post not found"), nil
	}

	// Increment view count atomically
	go h.incrementViewCount(post.ID, getSessionID(request))

	// Get author
	var author models.Author
	_ = h.db.Model(&models.Author{}).
		Where("ID", "=", post.AuthorID).
		First(&author)

	// Get category
	var category models.Category
	if post.CategoryID != "" {
		_ = h.db.Model(&models.Category{}).
			Where("ID", "=", post.CategoryID).
			First(&category)
	}

	// Build response
	response := map[string]any{
		"post":     post,
		"author":   author,
		"category": category,
	}

	return successResponse(http.StatusOK, response), nil
}

// createPost creates a new blog post
func (h *PostHandler) createPost(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Check authorization
	authorID := getAuthorID(request)
	if authorID == "" {
		return errorResponse(http.StatusUnauthorized, "Authorization required"), nil
	}

	// Parse request
	var req struct {
		Metadata      map[string]string `json:"metadata"`
		Title         string            `json:"title"`
		Content       string            `json:"content"`
		Excerpt       string            `json:"excerpt"`
		CategoryID    string            `json:"category_id"`
		Status        string            `json:"status"`
		FeaturedImage string            `json:"featured_image"`
		Tags          []string          `json:"tags"`
	}

	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body"), nil
	}

	// Validate
	if req.Title == "" || req.Content == "" {
		return errorResponse(http.StatusBadRequest, "Title and content are required"), nil
	}

	// Generate slug
	slug := generateSlug(req.Title)

	// Check if slug exists
	var existing models.Post
	err := h.db.Model(&models.Post{}).
		Index("gsi-slug").
		Where("Slug", "=", slug).
		First(&existing)

	if err == nil {
		// Slug exists, append number
		for i := 2; i < 100; i++ {
			testSlug := fmt.Sprintf("%s-%d", slug, i)
			err = h.db.Model(&models.Post{}).
				Index("gsi-slug").
				Where("Slug", "=", testSlug).
				First(&existing)
			if stdErrors.Is(err, errors.ErrItemNotFound) {
				slug = testSlug
				break
			}
		}
	}

	// Create post
	post := &models.Post{
		ID:            uuid.New().String(),
		Slug:          slug,
		AuthorID:      authorID,
		Title:         req.Title,
		Content:       req.Content,
		Excerpt:       req.Excerpt,
		CategoryID:    req.CategoryID,
		Tags:          req.Tags,
		Status:        req.Status,
		FeaturedImage: req.FeaturedImage,
		Metadata:      req.Metadata,
		ViewCount:     0,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Version:       1,
	}

	if post.Status == "" {
		post.Status = models.PostStatusDraft
	}

	if post.Status == models.PostStatusPublished {
		post.PublishedAt = time.Now()
	}

	// Start transaction
	err = h.db.Transaction(func(tx *core.Tx) error {
		// Create post
		if err := tx.Model(post).Create(); err != nil {
			return fmt.Errorf("failed to create post: %w", err)
		}

		// Create search index
		if post.Status == models.PostStatusPublished {
			searchIndex := &models.SearchIndex{
				ID:          fmt.Sprintf("post-%s", post.ID),
				ContentType: "post",
				SearchTerms: strings.ToLower(fmt.Sprintf("%s %s", post.Title, strings.Join(post.Tags, " "))),
				PostID:      post.ID,
				Title:       post.Title,
				Excerpt:     post.Excerpt,
				Tags:        post.Tags,
				UpdatedAt:   time.Now(),
			}
			if err := tx.Model(searchIndex).Create(); err != nil {
				// Non-critical, don't rollback
				fmt.Printf("Failed to create search index: %v\n", err)
			}
		}

		return nil
	})

	if err != nil {
		return errorResponse(http.StatusInternalServerError, fmt.Sprintf("Failed to create post: %v", err)), nil
	}

	// Update author and category post counts using atomic increments with UpdateBuilder
	go func() {
		// Update author post count atomically
		if err := h.db.Model(&models.Author{
			ID: authorID,
		}).UpdateBuilder().
			Increment("PostCount").
			Set("UpdatedAt", time.Now()).
			Execute(); err != nil {
			fmt.Printf("Failed to update author post count: %v\n", err)
		}

		// Update category post count if category is specified
		if req.CategoryID != "" {
			if err := h.db.Model(&models.Category{
				ID: req.CategoryID,
			}).UpdateBuilder().
				Increment("PostCount").
				Set("UpdatedAt", time.Now()).
				Execute(); err != nil {
				fmt.Printf("Failed to update category post count: %v\n", err)
			}
		}
	}()

	return successResponse(http.StatusCreated, post), nil
}

// updatePost updates an existing post
func (h *PostHandler) updatePost(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	postID := request.PathParameters["id"]
	if postID == "" {
		return errorResponse(http.StatusBadRequest, "Post ID is required"), nil
	}

	// Check authorization
	authorID := getAuthorID(request)
	if authorID == "" {
		return errorResponse(http.StatusUnauthorized, "Authorization required"), nil
	}

	// Get existing post
	var post models.Post
	err := h.db.Model(&models.Post{}).
		Where("ID", "=", postID).
		First(&post)

	if err != nil {
		if stdErrors.Is(err, errors.ErrItemNotFound) {
			return errorResponse(http.StatusNotFound, "Post not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch post"), nil
	}

	// Check permission
	if post.AuthorID != authorID && !isAdmin(request) {
		return errorResponse(http.StatusForbidden, "Permission denied"), nil
	}

	// Parse update request
	var req struct {
		Metadata      map[string]string `json:"metadata"`
		Title         string            `json:"title"`
		Content       string            `json:"content"`
		Excerpt       string            `json:"excerpt"`
		CategoryID    string            `json:"category_id"`
		Status        string            `json:"status"`
		FeaturedImage string            `json:"featured_image"`
		Tags          []string          `json:"tags"`
	}

	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body"), nil
	}

	// Build updates
	updates := map[string]any{
		"UpdatedAt": time.Now(),
	}

	if req.Title != "" && req.Title != post.Title {
		updates["Title"] = req.Title
		// Update slug if title changed
		newSlug := generateSlug(req.Title)
		if newSlug != post.Slug {
			// Check if new slug is available
			var existing models.Post
			err = h.db.Model(&models.Post{}).
				Index("gsi-slug").
				Where("Slug", "=", newSlug).
				First(&existing)
			if stdErrors.Is(err, errors.ErrItemNotFound) {
				updates["Slug"] = newSlug
			}
		}
	}

	if req.Content != "" {
		updates["Content"] = req.Content
	}
	if req.Excerpt != "" {
		updates["Excerpt"] = req.Excerpt
	}
	if req.CategoryID != post.CategoryID {
		updates["CategoryID"] = req.CategoryID
	}
	if len(req.Tags) > 0 {
		updates["Tags"] = req.Tags
	}
	if req.Status != "" && req.Status != post.Status {
		updates["Status"] = req.Status
		if req.Status == models.PostStatusPublished && post.Status != models.PostStatusPublished {
			updates["PublishedAt"] = time.Now()
		}
	}
	if req.FeaturedImage != "" {
		updates["FeaturedImage"] = req.FeaturedImage
	}
	if req.Metadata != nil {
		updates["Metadata"] = req.Metadata
	}

	// Update post with optimistic locking
	updateBuilder := h.db.Model(&models.Post{}).
		Where("ID", "=", postID).
		Where("Version", "=", post.Version).
		UpdateBuilder()

	// Apply all updates
	for field, value := range updates {
		switch field {
		case "Title":
			updateBuilder = updateBuilder.Set("Title", value)
		case "Content":
			updateBuilder = updateBuilder.Set("Content", value)
		case "Excerpt":
			updateBuilder = updateBuilder.Set("Excerpt", value)
		case "CategoryID":
			updateBuilder = updateBuilder.Set("CategoryID", value)
		case "Tags":
			updateBuilder = updateBuilder.Set("Tags", value)
		case "Status":
			updateBuilder = updateBuilder.Set("Status", value)
		case "FeaturedImage":
			updateBuilder = updateBuilder.Set("FeaturedImage", value)
		case "Metadata":
			updateBuilder = updateBuilder.Set("Metadata", value)
		case "UpdatedAt":
			updateBuilder = updateBuilder.Set("UpdatedAt", value)
		case "PublishedAt":
			updateBuilder = updateBuilder.Set("PublishedAt", value)
		case "Slug":
			updateBuilder = updateBuilder.Set("Slug", value)
		}
	}

	// Increment version for optimistic locking
	updateBuilder = updateBuilder.Increment("Version")

	err = updateBuilder.Execute()

	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return errorResponse(http.StatusConflict, "Post was modified by another user"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to update post"), nil
	}

	// Update search index if published
	if req.Status == models.PostStatusPublished || post.Status == models.PostStatusPublished {
		searchIndex := &models.SearchIndex{
			ID:          fmt.Sprintf("post-%s", post.ID),
			ContentType: "post",
			SearchTerms: strings.ToLower(fmt.Sprintf("%s %s", req.Title, strings.Join(req.Tags, " "))),
			PostID:      post.ID,
			Title:       req.Title,
			Excerpt:     req.Excerpt,
			Tags:        req.Tags,
			UpdatedAt:   time.Now(),
		}
		_ = h.db.Model(searchIndex).Create() // Update or create
	}

	// Return updated post
	post.UpdatedAt = updates["UpdatedAt"].(time.Time)
	for k, v := range updates {
		switch k {
		case "Title":
			post.Title = v.(string)
		case "Content":
			post.Content = v.(string)
		case "Status":
			post.Status = v.(string)
		}
	}

	return successResponse(http.StatusOK, post), nil
}

// deletePost deletes a blog post
func (h *PostHandler) deletePost(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	postID := request.PathParameters["id"]
	if postID == "" {
		return errorResponse(http.StatusBadRequest, "Post ID is required"), nil
	}

	// Check authorization
	authorID := getAuthorID(request)
	if authorID == "" {
		return errorResponse(http.StatusUnauthorized, "Authorization required"), nil
	}

	// Get post
	var post models.Post
	err := h.db.Model(&models.Post{}).
		Where("ID", "=", postID).
		First(&post)

	if err != nil {
		if stdErrors.Is(err, errors.ErrItemNotFound) {
			return errorResponse(http.StatusNotFound, "Post not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch post"), nil
	}

	// Check permission
	if post.AuthorID != authorID && !isAdmin(request) {
		return errorResponse(http.StatusForbidden, "Permission denied"), nil
	}

	// Soft delete by updating status
	err = h.db.Model(&models.Post{}).
		Where("ID", "=", postID).
		UpdateBuilder().
		Set("Status", models.PostStatusArchived).
		Set("UpdatedAt", time.Now()).
		Execute()

	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to delete post"), nil
	}

	// Delete search index
	_ = h.db.Model(&models.SearchIndex{}).
		Where("ID", "=", fmt.Sprintf("post-%s", postID)).
		Delete()

	return successResponse(http.StatusOK, map[string]string{
		"message": "Post deleted successfully",
	}), nil
}

// Helper functions

func (h *PostHandler) incrementViewCount(postID, sessionID string) {
	// Track unique views using session
	if sessionID != "" {
		var session models.Session
		err := h.db.Model(&models.Session{}).
			Where("ID", "=", sessionID).
			First(&session)

		if err == nil {
			// Check if already viewed
			for _, viewedID := range session.PostsViewed {
				if viewedID == postID {
					return // Already viewed
				}
			}
		}
	}

	// Get post to have required fields for UpdateBuilder
	var post models.Post
	if err := h.db.Model(&models.Post{}).
		Where("ID", "=", postID).
		First(&post); err != nil {
		fmt.Printf("Failed to get post %s: %v\n", postID, err)
		return
	}

	// Increment view count atomically using UpdateBuilder
	if err := h.db.Model(&models.Post{
		ID:       postID,
		AuthorID: post.AuthorID, // Required for composite key
	}).UpdateBuilder().
		Increment("ViewCount").
		Execute(); err != nil {
		fmt.Printf("Failed to increment view count for post %s: %v\n", postID, err)
		return
	}

	// Track view for analytics
	view := &models.PostView{
		ID:        fmt.Sprintf("%s:%d", postID, time.Now().UnixNano()),
		PostID:    postID,
		Timestamp: time.Now(),
		SessionID: sessionID,
		TTL:       time.Now().Add(90 * 24 * time.Hour),
	}
	_ = h.db.Model(view).Create()

	// Update session if exists
	if sessionID != "" {
		// Get the session again to have the latest data
		var latestSession models.Session
		if err := h.db.Model(&models.Session{}).
			Where("ID", "=", sessionID).
			First(&latestSession); err == nil {
			// Add the post to viewed list
			latestSession.PostsViewed = append(latestSession.PostsViewed, postID)
			// Update the session with the new posts viewed list
			_ = h.db.Model(&models.Session{
				ID: sessionID,
			}).UpdateBuilder().
				Set("PostsViewed", latestSession.PostsViewed).
				Execute()
		}
	}
}

func generateSlug(title string) string {
	// Convert to lowercase
	slug := strings.ToLower(title)

	// Replace non-alphanumeric characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug = reg.ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Limit length
	if len(slug) > 100 {
		slug = slug[:100]
	}

	return slug
}

func getAuthorID(request events.APIGatewayProxyRequest) string {
	// Extract from JWT claims or headers
	// This is a simplified version
	return request.Headers["X-Author-ID"]
}

func getSessionID(request events.APIGatewayProxyRequest) string {
	return request.Headers["X-Session-ID"]
}

func isAuthorized(request events.APIGatewayProxyRequest) bool {
	return getAuthorID(request) != ""
}

func isAdmin(request events.APIGatewayProxyRequest) bool {
	return request.Headers["X-Author-Role"] == models.RoleAdmin
}

func successResponse(statusCode int, data any) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(map[string]any{
		"success": true,
		"data":    data,
	})

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
		},
		Body: string(body),
	}
}

func errorResponse(statusCode int, message string) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(map[string]any{
		"success": false,
		"error":   message,
	})

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
		},
		Body: string(body),
	}
}
