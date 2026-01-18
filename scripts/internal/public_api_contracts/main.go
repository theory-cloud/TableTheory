package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

func main() {
	type model struct {
		_ struct{} `theorydb:"naming:snake_case"`

		ID        string `theorydb:"pk"`
		SK        string `theorydb:"sk"`
		UserID    string
		CreatedAt time.Time `theorydb:"created_at"`
		Custom    string    `theorydb:"attr:custom_name"`
	}

	item := map[string]types.AttributeValue{
		"id":          &types.AttributeValueMemberS{Value: "p1"},
		"sk":          &types.AttributeValueMemberS{Value: "s1"},
		"user_id":     &types.AttributeValueMemberS{Value: "u1"},
		"created_at":  &types.AttributeValueMemberS{Value: "2020-01-01T00:00:00Z"},
		"custom_name": &types.AttributeValueMemberS{Value: "c"},
	}

	var out model
	if err := theorydb.UnmarshalItem(item, &out); err != nil {
		fmt.Fprintf(os.Stderr, "public-api-contracts: unexpected error unmarshalling model\n")
		os.Exit(1)
	}
	if out.ID != "p1" || out.SK != "s1" || out.UserID != "u1" || out.Custom != "c" {
		fmt.Fprintf(os.Stderr, "public-api-contracts: unexpected field values after unmarshal\n")
		os.Exit(1)
	}
	if !out.CreatedAt.Equal(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
		fmt.Fprintf(os.Stderr, "public-api-contracts: unexpected CreatedAt after unmarshal\n")
		os.Exit(1)
	}

	type encryptedModel struct {
		_ struct{} `theorydb:"naming:snake_case"`

		Secret string `theorydb:"encrypted,attr:secret"`
	}

	encryptedItem := map[string]types.AttributeValue{
		"secret": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"v":     &types.AttributeValueMemberN{Value: "1"},
			"edk":   &types.AttributeValueMemberB{Value: []byte("edk")},
			"nonce": &types.AttributeValueMemberB{Value: []byte("nonce")},
			"ct":    &types.AttributeValueMemberB{Value: []byte("ct")},
		}},
	}

	var encryptedOut encryptedModel
	err := theorydb.UnmarshalItem(encryptedItem, &encryptedOut)
	if err == nil || !errors.Is(err, customerrors.ErrEncryptionNotConfigured) {
		fmt.Fprintf(os.Stderr, "public-api-contracts: expected encrypted unmarshal to fail closed\n")
		os.Exit(1)
	}

	fmt.Println("public-api-contracts: ok")
}
