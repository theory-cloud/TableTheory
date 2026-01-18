package testing_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

// User represents a user model
type User struct {
	ID        string `theorydb:"pk"`
	Email     string `theorydb:"sk"`
	Name      string
	Status    string
	CreatedAt string
}

// UserService is a service that uses TableTheory
type UserService struct {
	db core.DB
}

// NewUserService creates a new user service
func NewUserService(db core.DB) *UserService {
	return &UserService{db: db}
}

// GetUser retrieves a user by ID
func (s *UserService) GetUser(id string) (*User, error) {
	var user User
	err := s.db.Model(&User{}).Where("ID", "=", id).First(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetActiveUsers retrieves all active users
func (s *UserService) GetActiveUsers(limit int) ([]User, error) {
	var users []User
	err := s.db.Model(&User{}).
		Where("Status", "=", "active").
		OrderBy("CreatedAt", "DESC").
		Limit(limit).
		All(&users)
	return users, err
}

// CreateUser creates a new user
func (s *UserService) CreateUser(user *User) error {
	return s.db.Model(user).Create()
}

// DeactivateUser deactivates a user
func (s *UserService) DeactivateUser(id string) error {
	return s.db.Model(&User{ID: id}).
		UpdateBuilder().
		Set("Status", "inactive").
		Execute()
}

// TestGetUser demonstrates testing a simple query
func TestGetUser(t *testing.T) {
	// Create mocks
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Setup expectations
	mockDB.On("Model", &User{}).Return(mockQuery)
	mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		// Populate the result
		user := args.Get(0).(*User)
		user.ID = "123"
		user.Email = "john@example.com"
		user.Name = "John Doe"
		user.Status = "active"
	}).Return(nil)

	// Create service with mock
	service := NewUserService(mockDB)

	// Execute
	user, err := service.GetUser("123")

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "123", user.ID)
	assert.Equal(t, "John Doe", user.Name)
	assert.Equal(t, "john@example.com", user.Email)
	assert.Equal(t, "active", user.Status)

	// Verify all expectations were met
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestGetUser_NotFound demonstrates testing error cases
func TestGetUser_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Setup to return an error
	mockDB.On("Model", &User{}).Return(mockQuery)
	mockQuery.On("Where", "ID", "=", "999").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(errors.New("user not found"))

	service := NewUserService(mockDB)

	// Execute
	user, err := service.GetUser("999")

	// Assert
	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "user not found")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestGetActiveUsers demonstrates testing complex queries
func TestGetActiveUsers(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Setup chained method calls
	mockDB.On("Model", &User{}).Return(mockQuery)
	mockQuery.On("Where", "Status", "=", "active").Return(mockQuery)
	mockQuery.On("OrderBy", "CreatedAt", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		users := args.Get(0).(*[]User)
		*users = []User{
			{ID: "1", Name: "Alice", Status: "active", CreatedAt: "2024-01-02"},
			{ID: "2", Name: "Bob", Status: "active", CreatedAt: "2024-01-01"},
		}
	}).Return(nil)

	service := NewUserService(mockDB)

	// Execute
	users, err := service.GetActiveUsers(10)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, users, 2)
	assert.Equal(t, "Alice", users[0].Name)
	assert.Equal(t, "Bob", users[1].Name)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestCreateUser demonstrates testing create operations
func TestCreateUser(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	user := &User{
		ID:     "new-123",
		Email:  "new@example.com",
		Name:   "New User",
		Status: "active",
	}

	// Setup expectations
	mockDB.On("Model", user).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	service := NewUserService(mockDB)

	// Execute
	err := service.CreateUser(user)

	// Assert
	assert.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// TestDeactivateUser demonstrates testing update operations
func TestDeactivateUser(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	// Setup expectations
	mockDB.On("Model", &User{ID: "123"}).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", "Status", "inactive").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Execute").Return(nil)

	service := NewUserService(mockDB)

	// Execute
	err := service.DeactivateUser("123")

	// Assert
	assert.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdateBuilder.AssertExpectations(t)
}

// TestWithMatchedBy demonstrates using custom matchers
func TestWithMatchedBy(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Use MatchedBy for more flexible matching
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		_, ok := model.(*User)
		return ok
	})).Return(mockQuery)

	mockQuery.On("Where", "ID", "=", mock.MatchedBy(func(id any) bool {
		strID, ok := id.(string)
		return ok && len(strID) > 0
	})).Return(mockQuery)

	mockQuery.On("First", mock.Anything).Return(nil)

	service := NewUserService(mockDB)

	// This will match even with different IDs
	_, err := service.GetUser("any-id")
	assert.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
