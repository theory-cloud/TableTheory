package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
)

type EncryptedRecord struct {
	PK     string `theorydb:"pk" json:"pk"`
	SK     string `theorydb:"sk" json:"sk"`
	Secret string `theorydb:"encrypted" json:"secret"`
}

func (EncryptedRecord) TableName() string {
	if name := os.Getenv("DYNAMORM_ENCRYPTION_EXAMPLE_TABLE"); name != "" {
		return name
	}
	return "theorydb-encryption-example"
}

func main() {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	kmsKeyARN := os.Getenv("KMS_KEY_ARN")
	if kmsKeyARN == "" {
		log.Fatal("KMS_KEY_ARN is required to run this example")
	}

	db, err := tabletheory.New(session.Config{
		Region:    region,
		KMSKeyARN: kmsKeyARN,
	})
	if err != nil {
		log.Fatalf("init theorydb: %v", err)
	}

	if os.Getenv("DYNAMORM_ENCRYPTION_CREATE_TABLE") == "1" {
		if err := db.CreateTable(&EncryptedRecord{}); err != nil {
			log.Printf("CreateTable failed (table may already exist): %v", err)
		}
	}

	record := &EncryptedRecord{
		PK:     "record#1",
		SK:     "v1",
		Secret: "hello from theorydb",
	}

	if err := db.Model(record).Create(); err != nil {
		log.Fatalf("create: %v", err)
	}

	var out EncryptedRecord
	if err := db.Model(&EncryptedRecord{}).
		Where("PK", "=", record.PK).
		Where("SK", "=", record.SK).
		First(&out); err != nil {
		log.Fatalf("read: %v", err)
	}

	if out.Secret != record.Secret {
		log.Fatalf("decrypt assertion failed: read back %q, want %q", out.Secret, record.Secret)
	}
	fmt.Printf("decrypted secret: %q\n", out.Secret)
	fmt.Printf("plaintext round-trip: %q == %q\n", out.Secret, record.Secret)

	// Read the item without TableTheory to prove the stored attribute is a KMS
	// envelope, not plaintext: the ciphertext bytes must not contain the secret,
	// and the envelope must carry the data key, nonce and ciphertext fields.
	stored, err := readRawAttribute(context.Background(), region, record.TableName(), record.PK, record.SK, "secret")
	if err != nil {
		log.Fatalf("read stored attribute: %v", err)
	}
	envelope, ok := stored.(*types.AttributeValueMemberM)
	if !ok {
		log.Fatalf("stored attribute is %T, want an encrypted envelope map", stored)
	}
	for _, field := range []string{"v", "edk", "nonce", "ct"} {
		if _, present := envelope.Value[field]; !present {
			log.Fatalf("stored envelope is missing %q", field)
		}
	}
	ct, ok := envelope.Value["ct"].(*types.AttributeValueMemberB)
	if !ok || len(ct.Value) == 0 {
		log.Fatalf("stored envelope ciphertext is not a non-empty binary value")
	}
	if bytes.Contains(ct.Value, []byte(record.Secret)) {
		log.Fatalf("stored ciphertext contains the plaintext secret")
	}
	edk := envelope.Value["edk"].(*types.AttributeValueMemberB)
	nonce := envelope.Value["nonce"].(*types.AttributeValueMemberB)
	version := envelope.Value["v"].(*types.AttributeValueMemberN)
	fmt.Printf(
		"stored attribute is ciphertext: envelope v=%s edk=%dB nonce=%dB ct=%dB (no plaintext)\n",
		version.Value, len(edk.Value), len(nonce.Value), len(ct.Value),
	)
	fmt.Println("OK: KMS envelope round-trip encrypted on write and decrypted on read")
}

func readRawAttribute(ctx context.Context, region, table, pk, sk, attribute string) (types.AttributeValue, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	client := dynamodb.NewFromConfig(cfg)
	result, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("dynamodb GetItem: %w", err)
	}
	value, ok := result.Item[attribute]
	if !ok {
		return nil, fmt.Errorf("item has no %q attribute", attribute)
	}
	return value, nil
}
