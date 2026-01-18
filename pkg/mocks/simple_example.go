// Package mocks - Simple TableTheory Mocking Example
//
// This file demonstrates the basics of TableTheory mocking with a simple,
// real-world example that's easy to understand and follow.

package mocks

import (
	"fmt"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// 🏗️ STEP 1: Define your model
// This is just a regular TableTheory model
type User struct {
	ID    string `theorydb:"pk"`
	Email string
	Name  string
	Age   int
}

// 🚀 STEP 2: Create a service that uses TableTheory
// This is the real business logic you want to test
type UserService struct {
	db core.DB // This is the TableTheory database interface
}

func NewUserService(db core.DB) *UserService {
	return &UserService{db: db}
}

// GetUser fetches a user by ID
func (s *UserService) GetUser(id string) (*User, error) {
	var user User
	err := s.db.Model(&User{}).Where("ID", "=", id).First(&user)
	if err != nil {
		return nil, fmt.Errorf("failed to get user %s: %w", id, err)
	}
	return &user, nil
}

// CreateUser creates a new user
func (s *UserService) CreateUser(user *User) error {
	return s.db.Model(user).Create()
}

// GetActiveUsers gets all users over 18
func (s *UserService) GetActiveUsers() ([]User, error) {
	var users []User
	err := s.db.Model(&User{}).
		Where("Age", ">=", 18).
		OrderBy("Name", "ASC").
		All(&users)
	return users, err
}

// UpdateUserEmail updates a user's email
func (s *UserService) UpdateUserEmail(id, newEmail string) error {
	return s.db.Model(&User{}).
		Where("ID", "=", id).
		UpdateBuilder().
		Set("Email", newEmail).
		Execute()
}

// 🧪 STEP 3: Test the service using mocks
// See simple_example_test.go for the complete test examples

/*
Package Usage Summary:

1. Import the mocks: import "github.com/theory-cloud/tabletheory/pkg/mocks"

2. Create mock instances:
   mockDB := new(mocks.MockDB)
   mockQuery := new(mocks.MockQuery)

3. Set up expectations (what should be called):
   mockDB.On("Model", &User{}).Return(mockQuery)
   mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
   mockQuery.On("First", mock.Anything).Return(nil)

4. Use your service with the mock:
   service := NewUserService(mockDB)
   user, err := service.GetUser("123")

5. Verify expectations were met:
   mockDB.AssertExpectations(t)
   mockQuery.AssertExpectations(t)

Key Points:
- Mocks let you test your business logic without a real database
- Chain methods return the mock itself: Where().OrderBy().All()
- Use mock.Anything when you don't care about exact arguments
- Use mock.Run to populate output parameters
- Always verify expectations were met
*/
