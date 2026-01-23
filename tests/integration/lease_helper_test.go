package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/lease"
)

var leaseContractTableName string

type leaseContractModel struct {
	_ struct{} `theorydb:"naming:snake_case"`

	PK string `theorydb:"pk,attr:pk" json:"pk"`
	SK string `theorydb:"sk,attr:sk" json:"sk"`

	LeaseToken     string `theorydb:"attr:lease_token" json:"lease_token"`
	LeaseExpiresAt int64  `theorydb:"attr:lease_expires_at" json:"lease_expires_at"`
	TTL            int64  `theorydb:"ttl,attr:ttl,omitempty" json:"ttl,omitempty"`
}

func (leaseContractModel) TableName() string {
	return leaseContractTableName
}

func TestLeaseManager_TwoContenders(t *testing.T) {
	tc := InitTestDB(t)

	leaseContractTableName = fmt.Sprintf("lease_contract_%d", time.Now().UnixNano())
	tc.CreateTable(t, &leaseContractModel{})

	ctx := context.Background()

	mgr1, err := lease.NewManager(
		tc.DynamoDBClient,
		leaseContractTableName,
		lease.WithNow(func() time.Time { return time.Unix(1000, 0) }),
		lease.WithTokenGenerator(func() string { return "tok1" }),
		lease.WithTTLBuffer(10*time.Second),
	)
	require.NoError(t, err)

	mgr2, err := lease.NewManager(
		tc.DynamoDBClient,
		leaseContractTableName,
		lease.WithNow(func() time.Time { return time.Unix(1000, 0) }),
		lease.WithTokenGenerator(func() string { return "tok2" }),
		lease.WithTTLBuffer(10*time.Second),
	)
	require.NoError(t, err)

	got1, err := mgr1.Acquire(ctx, "CACHE#A", 30*time.Second)
	require.NoError(t, err)
	require.Equal(t, "tok1", got1.Token)

	_, err = mgr2.Acquire(ctx, "CACHE#A", 30*time.Second)
	require.Error(t, err)
	require.True(t, lease.IsLeaseHeld(err))

	mgr2Late, err := lease.NewManager(
		tc.DynamoDBClient,
		leaseContractTableName,
		lease.WithNow(func() time.Time { return time.Unix(2000, 0) }),
		lease.WithTokenGenerator(func() string { return "tok2" }),
		lease.WithTTLBuffer(10*time.Second),
	)
	require.NoError(t, err)

	got2, err := mgr2Late.Acquire(ctx, "CACHE#A", 30*time.Second)
	require.NoError(t, err)
	require.Equal(t, "tok2", got2.Token)

	_, err = mgr1.Refresh(ctx, *got1, 30*time.Second)
	require.Error(t, err)
	require.True(t, lease.IsLeaseNotOwned(err))
}
