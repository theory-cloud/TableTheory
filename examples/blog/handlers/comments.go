package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/examples/blog/models"
	"github.com/theory-cloud/tabletheory/examples/blog/services"
	"github.com/theory-cloud/tabletheory/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

// CommentHandler handles blog comment operations
type CommentHandler struct {
	db                  core.ExtendedDB
	notificationService *services.NotificationService
}

// commentNode represents a comment with its children
type commentNode struct {
	*models.Comment
	Children []*commentNode `json:"children,omitempty"`
}

// NewCommentHandler creates a new comment handler
func NewCommentHandler() (*CommentHandler, error) {
	db, err := theorydb.New(theorydb.Config{
		Region: "us-east-1",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamoDB: %w", err)
	}

	// Register models
	db.Model(&models.Comment{})
	db.Model(&models.Post{})

	// Initialize notification service
	notificationService := services.NewNotificationService(5)

	// Configure email provider
	emailConfig := services.EmailConfig{
		SMTPHost:     os.Getenv("SMTP_HOST"),
		SMTPPort:     os.Getenv("SMTP_PORT"),
		SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		FromEmail:    os.Getenv("FROM_EMAIL"),
		FromName:     "Blog Notifications",
		TestMode:     os.Getenv("NOTIFICATION_TEST_MODE") == "true",
	}
	if emailConfig.SMTPHost == "" {
		// Use test mode if SMTP is not configured
		emailConfig.TestMode = true
	}
	emailProvider := services.NewEmailProvider(emailConfig)
	notificationService.RegisterProvider(emailProvider)

	// Configure webhook provider
	webhookConfig := services.WebhookConfig{
		DefaultWebhookURL: os.Getenv("WEBHOOK_URL"),
		SigningSecret:     os.Getenv("WEBHOOK_SECRET"),
		TestMode:          os.Getenv("NOTIFICATION_TEST_MODE") == "true",
	}
	webhookProvider := services.NewWebhookProvider(webhookConfig)
	notificationService.RegisterProvider(webhookProvider)

	return &CommentHandler{
		db:                  db,
		notificationService: notificationService,
	}, nil
}

// HandleRequest routes requests to appropriate handlers
func (h *CommentHandler) HandleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Extract post ID from path
	postID := request.PathParameters["postId"]

	switch request.HTTPMethod {
	case "GET":
		return h.listComments(ctx, postID, request)
	case "POST":
		return h.createComment(ctx, postID, request)
	case "PUT":
		return h.moderateComment(ctx, request)
	case "DELETE":
		return h.deleteComment(ctx, request)
	default:
		return errorResponse(http.StatusMethodNotAllowed, "Method not allowed"), nil
	}
}

// listComments returns comments for a post with nested structure
func (h *CommentHandler) listComments(ctx context.Context, postID string, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if postID == "" {
		return errorResponse(http.StatusBadRequest, "Post ID is required"), nil
	}

	// Parse query parameters
	limit, _ := strconv.Atoi(request.QueryStringParameters["limit"])
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	status := request.QueryStringParameters["status"]
	if status == "" {
		status = models.CommentStatusApproved // Only show approved by default
	}

	// Build query
	query := h.db.Model(&models.Comment{}).
		Index("gsi-post").
		Where("PostID", "=", postID).
		OrderBy("CreatedAt", "ASC").
		Limit(limit)

	// Apply status filter for non-admins
	if !isAdmin(request) {
		query = query.Filter("Status", "=", models.CommentStatusApproved)
	} else if status != "all" {
		query = query.Filter("Status", "=", status)
	}

	// Execute query
	var comments []*models.Comment
	err := query.All(&comments)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch comments"), nil
	}

	// Build nested comment structure
	commentMap := make(map[string]*models.Comment)
	rootComments := make([]*models.Comment, 0)

	for _, comment := range comments {
		commentMap[comment.ID] = comment
		if comment.ParentID == "" {
			rootComments = append(rootComments, comment)
		}
	}

	// Build tree structure

	var buildTree func(parent *models.Comment) *commentNode
	buildTree = func(parent *models.Comment) *commentNode {
		node := &commentNode{
			Comment:  parent,
			Children: make([]*commentNode, 0),
		}

		// Find children
		for _, comment := range comments {
			if comment.ParentID == parent.ID {
				child := buildTree(comment)
				node.Children = append(node.Children, child)
			}
		}

		return node
	}

	// Build root nodes
	tree := make([]*commentNode, 0, len(rootComments))
	for _, root := range rootComments {
		tree = append(tree, buildTree(root))
	}

	// Get comment count for the post
	var post models.Post
	_ = h.db.Model(&models.Post{}).
		Where("ID", "=", postID).
		First(&post)

	response := map[string]any{
		"comments":      tree,
		"next_cursor":   "", // Cursor not supported in current implementation
		"has_more":      false,
		"total_count":   len(comments),
		"comment_count": countCommentsInTree(tree),
	}

	return successResponse(http.StatusOK, response), nil
}

// createComment creates a new comment on a post
func (h *CommentHandler) createComment(ctx context.Context, postID string, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if postID == "" {
		return errorResponse(http.StatusBadRequest, "Post ID is required"), nil
	}

	// Parse request
	var req struct {
		ParentID    string `json:"parent_id"`
		AuthorName  string `json:"author_name"`
		AuthorEmail string `json:"author_email"`
		Content     string `json:"content"`
	}

	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body"), nil
	}

	// Validate
	if req.Content == "" {
		return errorResponse(http.StatusBadRequest, "Content is required"), nil
	}

	if req.AuthorName == "" || req.AuthorEmail == "" {
		return errorResponse(http.StatusBadRequest, "Author name and email are required"), nil
	}

	// Check if post exists and is published
	var post models.Post
	err := h.db.Model(&models.Post{}).
		Where("ID", "=", postID).
		First(&post)

	if err != nil {
		if err == customerrors.ErrItemNotFound {
			return errorResponse(http.StatusNotFound, "Post not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch post"), nil
	}

	if post.Status != models.PostStatusPublished {
		return errorResponse(http.StatusForbidden, "Comments are not allowed on this post"), nil
	}

	// Check if parent comment exists (for nested comments)
	if req.ParentID != "" {
		var parentComment models.Comment
		err = h.db.Model(&models.Comment{}).
			Where("ID", "=", req.ParentID).
			Where("PostID", "=", postID).
			First(&parentComment)

		if err != nil {
			return errorResponse(http.StatusBadRequest, "Parent comment not found"), nil
		}

		// Limit nesting depth
		if strings.Count(parentComment.ParentID, ":") >= 2 {
			return errorResponse(http.StatusBadRequest, "Maximum nesting depth exceeded"), nil
		}
	}

	// Get author ID if authenticated
	authorID := getAuthorID(request)

	// Create comment
	comment := &models.Comment{
		ID:          uuid.New().String(),
		PostID:      postID,
		ParentID:    req.ParentID,
		AuthorID:    authorID,
		AuthorName:  req.AuthorName,
		AuthorEmail: req.AuthorEmail,
		Content:     req.Content,
		Status:      models.CommentStatusPending, // Moderate by default
		IPAddress:   request.RequestContext.Identity.SourceIP,
		UserAgent:   request.Headers["User-Agent"],
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Auto-approve if author is authenticated
	if authorID != "" {
		comment.Status = models.CommentStatusApproved
	}

	// Check for spam (simple check)
	if isSpam(comment.Content) {
		comment.Status = models.CommentStatusSpam
	}

	// Create comment
	if err := h.db.Model(comment).Create(); err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to create comment"), nil
	}

	// Send moderation notification if pending
	if comment.Status == models.CommentStatusPending {
		// Get the post details for the notification
		var post models.Post
		err := h.db.Model(&models.Post{}).
			Where("ID", "=", postID).
			First(&post)
		if err == nil {
			// Send notification asynchronously
			go func() {
				if err := h.notificationService.SendCommentModerationNotification(comment, &post); err != nil {
					// Log error but don't fail the request
					fmt.Printf("Failed to send moderation notification: %v\n", err)
				}
			}()
		}
	}

	response := map[string]any{
		"comment": comment,
		"message": "Comment submitted successfully",
	}

	if comment.Status == models.CommentStatusPending {
		response["message"] = "Comment submitted and awaiting moderation"
	}

	return successResponse(http.StatusCreated, response), nil
}

// moderateComment updates comment status (admin only)
func (h *CommentHandler) moderateComment(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Check authorization
	if !isAdmin(request) {
		return errorResponse(http.StatusForbidden, "Admin access required"), nil
	}

	commentID := request.PathParameters["commentId"]
	if commentID == "" {
		return errorResponse(http.StatusBadRequest, "Comment ID is required"), nil
	}

	// Parse request
	var req struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}

	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body"), nil
	}

	// Validate status
	validStatuses := []string{
		models.CommentStatusApproved,
		models.CommentStatusPending,
		models.CommentStatusSpam,
	}

	valid := false
	for _, s := range validStatuses {
		if req.Status == s {
			valid = true
			break
		}
	}

	if !valid {
		return errorResponse(http.StatusBadRequest, "Invalid status"), nil
	}

	// Get comment
	var comment models.Comment
	err := h.db.Model(&models.Comment{}).
		Where("ID", "=", commentID).
		First(&comment)

	if err != nil {
		if err == customerrors.ErrItemNotFound {
			return errorResponse(http.StatusNotFound, "Comment not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch comment"), nil
	}

	// Update status
	comment.Status = req.Status
	comment.UpdatedAt = time.Now()

	err = h.db.Model(&comment).Update("Status", "UpdatedAt")

	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to update comment"), nil
	}

	// Send notification to comment author if approved
	if req.Status == models.CommentStatusApproved && comment.Status != models.CommentStatusApproved {
		// Get the post details for the notification
		var post models.Post
		err := h.db.Model(&models.Post{}).
			Where("ID", "=", comment.PostID).
			First(&post)
		if err == nil {
			// Send notification asynchronously
			go func() {
				if err := h.notificationService.SendCommentApprovalNotification(&comment, &post); err != nil {
					// Log error but don't fail the request
					fmt.Printf("Failed to send approval notification: %v\n", err)
				}
			}()
		}
	}

	return successResponse(http.StatusOK, map[string]any{
		"message": "Comment moderated successfully",
		"status":  req.Status,
	}), nil
}

// deleteComment deletes a comment (admin or author only)
func (h *CommentHandler) deleteComment(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	commentID := request.PathParameters["commentId"]
	if commentID == "" {
		return errorResponse(http.StatusBadRequest, "Comment ID is required"), nil
	}

	// Get comment
	var comment models.Comment
	err := h.db.Model(&models.Comment{}).
		Where("ID", "=", commentID).
		First(&comment)

	if err != nil {
		if err == customerrors.ErrItemNotFound {
			return errorResponse(http.StatusNotFound, "Comment not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch comment"), nil
	}

	// Check permission
	authorID := getAuthorID(request)
	if comment.AuthorID != authorID && !isAdmin(request) {
		return errorResponse(http.StatusForbidden, "Permission denied"), nil
	}

	// Check if comment has children
	childCount, err := h.db.Model(&models.Comment{}).
		Index("gsi-post").
		Where("PostID", "=", comment.PostID).
		Where("ParentID", "=", commentID).
		Count()

	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to check for child comments"), nil
	}

	if childCount > 0 {
		// Soft delete - just update content
		comment.Content = "[Comment deleted]"
		comment.Status = "deleted"
		comment.UpdatedAt = time.Now()

		err = h.db.Model(&comment).Update("Content", "Status", "UpdatedAt")
	} else {
		// Hard delete
		err = h.db.Model(&models.Comment{}).
			Where("ID", "=", commentID).
			Delete()
	}

	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to delete comment"), nil
	}

	return successResponse(http.StatusOK, map[string]string{
		"message": "Comment deleted successfully",
	}), nil
}

// Helper functions

func countComments(nodes []*commentNode) int {
	count := 0
	for _, node := range nodes {
		count++
		count += countComments(node.Children)
	}
	return count
}

func countCommentsInTree(nodes []*commentNode) int {
	count := 0
	for _, node := range nodes {
		count++
		count += countComments(node.Children)
	}
	return count
}

func isSpam(content string) bool {
	// Simple spam detection
	spamWords := []string{
		"viagra", "cialis", "casino", "lottery", "click here",
		"buy now", "limited offer", "act now",
	}

	lowerContent := strings.ToLower(content)
	for _, word := range spamWords {
		if strings.Contains(lowerContent, word) {
			return true
		}
	}

	// Check for excessive links
	linkCount := strings.Count(lowerContent, "http://") + strings.Count(lowerContent, "https://")
	if linkCount > 3 {
		return true
	}

	// Check for excessive caps
	upperCount := 0
	for _, r := range content {
		if r >= 'A' && r <= 'Z' {
			upperCount++
		}
	}
	if float64(upperCount)/float64(len(content)) > 0.5 && len(content) > 10 {
		return true
	}

	return false
}
