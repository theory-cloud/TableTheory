package main

import (
	"fmt"
	"log"
	"os"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
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

	db, err := theorydb.New(session.Config{
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

	fmt.Printf("decrypted secret: %q\n", out.Secret)
}
