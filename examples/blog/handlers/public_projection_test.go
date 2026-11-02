package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/examples/blog/models"
	"github.com/theory-cloud/tabletheory/v6/examples/blog/services"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
	"github.com/theory-cloud/tabletheory/v6/pkg/testing/fakedb"
)

func newBlogTestDB(t *testing.T) core.ExtendedDB {
	t.Helper()

	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	for _, model := range []any{&models.Post{}, &models.Author{}, &models.Comment{}} {
		require.NoError(t, db.CreateTable(model))
	}
	return db
}

func seedBlogPosts(t *testing.T, db core.ExtendedDB) {
	t.Helper()

	require.NoError(t, db.Model(&models.Author{
		ID:       "author#1",
		Email:    "secret-author@example.com",
		Username: "jane",
		Name:     "Jane",
	}).Create())

	now := time.Now()
	require.NoError(t, db.Model(&models.Post{
		ID:          "post#published",
		Status:      models.PostStatusPublished,
		Title:       "Published",
		Slug:        "published",
		AuthorID:    "author#1",
		Content:     "public body",
		PublishedAt: now,
	}).Create())
	require.NoError(t, db.Model(&models.Post{
		ID:       "post#draft",
		Status:   models.PostStatusDraft,
		Title:    "Draft",
		Slug:     "draft",
		AuthorID: "author#1",
		Content:  "draft body",
	}).Create())
}

func TestListPostsNeverReturnsUnpublishedOrAuthorEmail(t *testing.T) {
	db := newBlogTestDB(t)
	seedBlogPosts(t, db)

	handler := &PostHandler{db: db}
	response, err := handler.listPosts(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:            "GET",
		QueryStringParameters: map[string]string{"status": "draft"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NotContains(t, response.Body, "secret-author@example.com")
	require.NotContains(t, response.Body, "Draft")
	require.Contains(t, response.Body, "Published")

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(response.Body), &payload))
	data := payload["data"].(map[string]any)
	posts := data["posts"].([]any)
	require.Len(t, posts, 1)
}

func TestGetPostBySlugHidesDrafts(t *testing.T) {
	db := newBlogTestDB(t)
	seedBlogPosts(t, db)

	handler := &PostHandler{db: db}
	response, err := handler.getPostBySlug(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:     "GET",
		PathParameters: map[string]string{"slug": "draft"},
		Headers:        map[string]string{"X-Author-ID": "author#1", "X-Author-Role": models.RoleAdmin},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestListCommentsOnlyReturnsApprovedAndOmitsPII(t *testing.T) {
	db := newBlogTestDB(t)
	now := time.Now()
	for _, comment := range []*models.Comment{
		{
			ID:          "comment#approved",
			PostID:      "post#published",
			AuthorName:  "Approved",
			AuthorEmail: "commenter@example.com",
			Content:     "approved body",
			Status:      models.CommentStatusApproved,
			IPAddress:   "203.0.113.10",
			UserAgent:   "test-agent",
			CreatedAt:   now,
		},
		{
			ID:          "comment#spam",
			PostID:      "post#published",
			AuthorName:  "Spam",
			AuthorEmail: "spammer@example.com",
			Content:     "spam body",
			Status:      models.CommentStatusSpam,
			IPAddress:   "203.0.113.20",
			UserAgent:   "spam-agent",
			CreatedAt:   now,
		},
	} {
		require.NoError(t, db.Model(comment).Create())
	}

	handler := &CommentHandler{db: db}
	response, err := handler.listComments(context.Background(), "post#published", events.APIGatewayProxyRequest{
		HTTPMethod:            "GET",
		QueryStringParameters: map[string]string{"status": "spam"},
		Headers:               map[string]string{"X-Author-Role": models.RoleAdmin},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Contains(t, response.Body, "approved body")
	require.NotContains(t, response.Body, "spam body")
	require.NotContains(t, response.Body, "commenter@example.com")
	require.NotContains(t, response.Body, "203.0.113.10")
	require.NotContains(t, response.Body, "test-agent")
}

func TestCreateCommentNeverExposesTheRootSentinel(t *testing.T) {
	db := newBlogTestDB(t)
	seedBlogPosts(t, db)
	handler := &CommentHandler{db: db, notificationService: services.NewNotificationService()}

	createComment := func(body string, authorizer map[string]any) map[string]any {
		t.Helper()
		response, err := handler.createComment(context.Background(), "post#published", events.APIGatewayProxyRequest{
			HTTPMethod:     "POST",
			PathParameters: map[string]string{"postId": "post#published"},
			Body:           body,
			RequestContext: events.APIGatewayProxyRequestContext{Authorizer: authorizer},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, response.StatusCode)

		var payload struct {
			Data struct {
				Comment map[string]any `json:"comment"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(response.Body), &payload))
		return payload.Data.Comment
	}

	root := createComment(
		`{"author_name":"Reader","author_email":"reader@example.com","content":"root comment"}`,
		nil,
	)
	_, exposedParent := root["parent_id"]
	require.False(t, exposedParent, "a root create response must not carry a parent_id: %v", root["parent_id"])
	require.Equal(t, models.CommentStatusPending, root["status"],
		"the create response must keep its moderation status")

	rootID, ok := root["id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, rootID)

	var stored models.Comment
	require.NoError(t, db.Model(&models.Comment{}).Where("ID", "=", rootID).First(&stored))
	require.Equal(t, rootCommentParentID, stored.ParentID,
		"the GSI sort key sentinel must stay stored")

	reply := createComment(
		fmt.Sprintf(`{"parent_id":%q,"author_name":"Jane","author_email":"jane@example.com","content":"reply"}`,
			rootID),
		authorizerContext("author#1", models.RoleAuthor),
	)
	require.Equal(t, rootID, reply["parent_id"], "a reply must keep its real parent id")
}
