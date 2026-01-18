// Package mocks provides mock implementations for TableTheory interfaces and AWS SDK operations
package mocks

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/mock"
)

// MockKMSClient provides a mock implementation of the AWS KMS client for
// testing TableTheory encryption flows without real AWS KMS calls.
//
// Example usage:
//
//	kmsMock := new(mocks.MockKMSClient)
//	kmsMock.On("GenerateDataKey", mock.Anything, mock.Anything, mock.Anything).
//	  Return(&kms.GenerateDataKeyOutput{Plaintext: key, CiphertextBlob: edk}, nil)
//
//	db, _ := theorydb.New(session.Config{
//	  Region:    "us-east-1",
//	  KMSKeyARN: "arn:aws:kms:us-east-1:111111111111:key/test",
//	  KMSClient: kmsMock,
//	})
type MockKMSClient struct {
	mock.Mock
}

func (m *MockKMSClient) GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*kms.GenerateDataKeyOutput)
	if !ok {
		panic("unexpected type: expected *kms.GenerateDataKeyOutput")
	}
	return output, args.Error(1)
}

func (m *MockKMSClient) Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*kms.DecryptOutput)
	if !ok {
		panic("unexpected type: expected *kms.DecryptOutput")
	}
	return output, args.Error(1)
}
