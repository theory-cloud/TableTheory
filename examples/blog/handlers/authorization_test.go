package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v6/examples/blog/models"
	"github.com/theory-cloud/tabletheory/v6/examples/blog/services"
)

func authorizerContext(authorID, role string) map[string]any {
	return map[string]any{
		authorizerAuthorIDKey: authorID,
		authorizerRoleKey:     role,
	}
}

func updatePostRequest(headers map[string]string, authorizer map[string]any) events.APIGatewayProxyRequest {
	return events.APIGatewayProxyRequest{
		HTTPMethod:     "PUT",
		PathParameters: map[string]string{"id": "post#published"},
		Headers:        headers,
		Body:           `{"content":"updated"}`,
		RequestContext: events.APIGatewayProxyRequestContext{Authorizer: authorizer},
	}
}

func TestProtectedPostUpdateRejectsSpoofedIdentityHeaders(t *testing.T) {
	db := newBlogTestDB(t)
	seedBlogPosts(t, db)
	handler := &PostHandler{db: db}

	response, err := handler.updatePost(context.Background(), updatePostRequest(map[string]string{
		"X-Author-ID":   "author#1",
		"X-Author-Role": models.RoleAdmin,
	}, nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode,
		"caller-controlled identity headers must not authenticate a request")
}

func TestProtectedPostUpdateAcceptsAuthorizerContext(t *testing.T) {
	db := newBlogTestDB(t)
	seedBlogPosts(t, db)
	handler := &PostHandler{db: db}

	response, err := handler.updatePost(context.Background(), updatePostRequest(nil,
		authorizerContext("author#1", models.RoleAuthor)))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func TestProtectedPostUpdateRejectsNonOwnerAuthorizerContext(t *testing.T) {
	db := newBlogTestDB(t)
	seedBlogPosts(t, db)
	handler := &PostHandler{db: db}

	response, err := handler.updatePost(context.Background(), updatePostRequest(nil,
		authorizerContext("author#2", models.RoleAuthor)))
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
}

func TestModerationRequiresAdminAuthorizerContext(t *testing.T) {
	db := newBlogTestDB(t)
	require.NoError(t, db.Model(&models.Comment{
		ID:        "comment#pending",
		PostID:    "post#published",
		Content:   "pending body",
		Status:    models.CommentStatusPending,
		CreatedAt: time.Now(),
	}).Create())
	handler := &CommentHandler{db: db}

	request := events.APIGatewayProxyRequest{
		HTTPMethod:     "PUT",
		PathParameters: map[string]string{"commentId": "comment#pending"},
		Body:           `{"status":"approved"}`,
		Headers:        map[string]string{"X-Author-Role": models.RoleAdmin},
	}

	response, err := handler.moderateComment(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode,
		"an X-Author-Role header must not grant admin access")

	request.RequestContext.Authorizer = authorizerContext("author#9", models.RoleAdmin)
	response, err = handler.moderateComment(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func TestHeaderIdentityRequiresExplicitOptIn(t *testing.T) {
	t.Setenv(allowHeaderIdentityEnv, "true")

	db := newBlogTestDB(t)
	seedBlogPosts(t, db)
	handler := &PostHandler{db: db}

	response, err := handler.updatePost(context.Background(), updatePostRequest(map[string]string{
		"X-Author-ID": "author#1",
	}, nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func TestCommentAuthorIdentityComesOnlyFromAuthorizerContext(t *testing.T) {
	db := newBlogTestDB(t)
	seedBlogPosts(t, db)
	handler := &CommentHandler{db: db, notificationService: services.NewNotificationService()}

	createComment := func(clickContext map[string]any) string {
		t.Helper()
		response, err := handler.createComment(context.Background(), "post#published", events.APIGatewayProxyRequest{
			HTTPMethod:     "POST",
			PathParameters: map[string]string{"postId": "post#published"},
			Body:           `{"author_name":"Jane","author_email":"jane@example.com","content":"hello"}`,
			Headers:        map[string]string{"X-Author-ID": "author#1"},
			RequestContext: events.APIGatewayProxyRequestContext{Authorizer: clickContext},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, response.StatusCode)

		var payload struct {
			Data struct {
				Comment struct {
					AuthorID string `json:"author_id"`
				} `json:"comment"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(response.Body), &payload))
		return payload.Data.Comment.AuthorID
	}

	require.Empty(t, createComment(nil), "a spoofed X-Author-ID must not become a comment author")
	require.Equal(t, "author#1", createComment(authorizerContext("author#1", models.RoleAuthor)))
}
