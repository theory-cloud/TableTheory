package fakedb_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	theorydberrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"github.com/theory-cloud/tabletheory/pkg/testing/fakedb"
)

type fakeUser struct {
	CreatedAt time.Time `theorydb:"created_at" json:"createdAt"`
	UpdatedAt time.Time `theorydb:"updated_at" json:"updatedAt"`
	PK        string    `theorydb:"pk" json:"PK"`
	SK        string    `theorydb:"sk" json:"SK"`
	EmailHash string    `json:"emailHash"`
	Nickname  string    `json:"nickname,omitempty"`
	Version   int64     `theorydb:"version" json:"version"`
	TTL       int64     `theorydb:"ttl,omitempty" json:"ttl,omitempty"`
}

func (fakeUser) TableName() string { return "fake_users" }

func TestNewWithClientUsesStatefulFake(t *testing.T) {
	now := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{
		Region: "us-east-1",
		Now:    func() time.Time { return now },
	}, fake)
	require.NoError(t, err)

	require.NoError(t, db.CreateTable(&fakeUser{}))
	require.NoError(t, db.Model(&fakeUser{
		PK:        "USER#1",
		SK:        "PROFILE",
		EmailHash: "test@example",
		Nickname:  "one",
		TTL:       1_700_000_000,
	}).Create())

	var queried []fakeUser
	err = db.Model(&fakeUser{}).
		Where("PK", "=", "USER#1").
		Where("SK", "begins_with", "PRO").
		Filter("emailHash", "=", "test@example").
		All(&queried)
	require.NoError(t, err)
	require.Len(t, queried, 1)
	require.Equal(t, "one", queried[0].Nickname)
	require.Equal(t, int64(1_700_000_000), queried[0].TTL)
	require.Equal(t, now, queried[0].CreatedAt)
	require.Equal(t, int64(0), queried[0].Version)

	require.NoError(t, db.Model(&fakeUser{
		PK:       "USER#1",
		SK:       "PROFILE",
		Nickname: "two",
		Version:  0,
	}).Update("nickname"))

	err = db.Model(&fakeUser{
		PK:       "USER#1",
		SK:       "PROFILE",
		Nickname: "stale",
		Version:  0,
	}).Update("nickname")
	require.Error(t, err)
	require.True(t, errors.Is(err, theorydberrors.ErrVersionConflict) || errors.Is(err, theorydberrors.ErrConditionFailed))

	var got fakeUser
	require.NoError(t, db.Model(&fakeUser{}).Where("PK", "=", "USER#1").Where("SK", "=", "PROFILE").First(&got))
	require.Equal(t, "two", got.Nickname)
	require.Equal(t, int64(1), got.Version)
}

func TestNewWithClientRejectsNilClient(t *testing.T) {
	_, err := tabletheory.NewWithClient(session.Config{}, nil)
	require.ErrorContains(t, err, "DynamoDB client is nil")
}
