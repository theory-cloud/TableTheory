package driver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

type deterministicEncryptionMaterial struct {
	mu sync.Mutex

	master    []byte
	model     string
	attribute string
	counter   uint64
}

func deterministicEncryptionForOptions(opts EncryptionOptions) (*deterministicKMSClient, io.Reader, error) {
	if opts.Provider != "deterministic" {
		return nil, nil, fmt.Errorf("unsupported scenario encryption provider: %q", opts.Provider)
	}
	if opts.Seed == "" {
		return nil, nil, fmt.Errorf("deterministic scenario encryption requires seed")
	}
	material := newDeterministicEncryptionMaterial(opts.Seed, "EncryptedRecord", "secret")
	return &deterministicKMSClient{material: material}, &deterministicNonceReader{material: material}, nil
}

func newDeterministicEncryptionMaterial(seed string, model string, attribute string) *deterministicEncryptionMaterial {
	sum := sha256.Sum256([]byte(seed))
	return &deterministicEncryptionMaterial{
		master:    sum[:],
		model:     model,
		attribute: attribute,
	}
}

func (m *deterministicEncryptionMaterial) nextBytes(label string, n int) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counter++
	mac := hmac.New(sha256.New, m.master)
	_, _ = mac.Write([]byte(fmt.Sprintf("%s|%d", label, m.counter)))
	return mac.Sum(nil)[:n]
}

func (m *deterministicEncryptionMaterial) dataKey(edk []byte) []byte {
	mac := hmac.New(sha256.New, m.master)
	_, _ = mac.Write(edk)
	return mac.Sum(nil)
}

type deterministicKMSClient struct {
	material *deterministicEncryptionMaterial
}

func (c *deterministicKMSClient) GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	_ = ctx
	_ = params
	_ = optFns

	label := fmt.Sprintf("edk|%s|%s", c.material.model, c.material.attribute)
	edk := c.material.nextBytes(label, 32)
	return &kms.GenerateDataKeyOutput{
		Plaintext:      c.material.dataKey(edk),
		CiphertextBlob: edk,
		KeyId:          aws.String("contract-deterministic"),
	}, nil
}

func (c *deterministicKMSClient) Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	_ = ctx
	_ = optFns
	if params == nil {
		return nil, fmt.Errorf("DecryptInput is nil")
	}
	return &kms.DecryptOutput{
		Plaintext: c.material.dataKey(params.CiphertextBlob),
		KeyId:     aws.String("contract-deterministic"),
	}, nil
}

type deterministicNonceReader struct {
	material *deterministicEncryptionMaterial
	buf      bytes.Buffer
}

func (r *deterministicNonceReader) Read(p []byte) (int, error) {
	for r.buf.Len() < len(p) {
		label := fmt.Sprintf("nonce|%s|%s", r.material.model, r.material.attribute)
		_, _ = r.buf.Write(r.material.nextBytes(label, 32))
	}
	return r.buf.Read(p)
}
