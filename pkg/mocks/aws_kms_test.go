package mocks

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestMockKMSClient_GenerateDataKey(t *testing.T) {
	m := new(MockKMSClient)
	want := &kms.GenerateDataKeyOutput{
		Plaintext:      []byte("plaintext"),
		CiphertextBlob: []byte("edk"),
	}

	m.On("GenerateDataKey", mock.Anything, mock.Anything, mock.Anything).
		Return(want, nil).
		Once()

	got, err := m.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{})
	require.NoError(t, err)
	require.Equal(t, want, got)
	m.AssertExpectations(t)
}

func TestMockKMSClient_GenerateDataKey_NilOutputReturnsError(t *testing.T) {
	m := new(MockKMSClient)
	wantErr := errors.New("kms down")

	m.On("GenerateDataKey", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, wantErr).
		Once()

	got, err := m.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{})
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, got)
	m.AssertExpectations(t)
}

func TestMockKMSClient_GenerateDataKey_PanicsOnWrongType(t *testing.T) {
	m := new(MockKMSClient)

	m.On("GenerateDataKey", mock.Anything, mock.Anything, mock.Anything).
		Return("bad-type", nil).
		Once()

	require.Panics(t, func() {
		_, err := m.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{})
		require.NoError(t, err)
	})
	m.AssertExpectations(t)
}

func TestMockKMSClient_Decrypt(t *testing.T) {
	m := new(MockKMSClient)
	want := &kms.DecryptOutput{Plaintext: []byte("plaintext")}

	m.On("Decrypt", mock.Anything, mock.Anything, mock.Anything).
		Return(want, nil).
		Once()

	got, err := m.Decrypt(context.Background(), &kms.DecryptInput{CiphertextBlob: []byte("edk")})
	require.NoError(t, err)
	require.Equal(t, want, got)
	m.AssertExpectations(t)
}

func TestMockKMSClient_Decrypt_NilOutputReturnsError(t *testing.T) {
	m := new(MockKMSClient)
	wantErr := errors.New("kms down")

	m.On("Decrypt", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, wantErr).
		Once()

	got, err := m.Decrypt(context.Background(), &kms.DecryptInput{CiphertextBlob: []byte("edk")})
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, got)
	m.AssertExpectations(t)
}

func TestMockKMSClient_Decrypt_PanicsOnWrongType(t *testing.T) {
	m := new(MockKMSClient)

	m.On("Decrypt", mock.Anything, mock.Anything, mock.Anything).
		Return("bad-type", nil).
		Once()

	require.Panics(t, func() {
		_, err := m.Decrypt(context.Background(), &kms.DecryptInput{CiphertextBlob: []byte("edk")})
		require.NoError(t, err)
	})
	m.AssertExpectations(t)
}
