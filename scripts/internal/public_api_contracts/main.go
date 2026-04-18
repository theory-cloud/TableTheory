package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

type BaseObject struct {
	ID   string
	Type string
	To   []string
}

type Activity struct {
	BaseObject
	Actor  string
	Object string
}

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
	if err := tabletheory.UnmarshalItem(item, &out); err != nil {
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

	expectedActivity := Activity{
		BaseObject: BaseObject{
			ID:   "https://example.com/activities/1",
			Type: "Create",
			To: []string{
				"https://www.w3.org/ns/activitystreams#Public",
				"https://example.com/users/alice/followers",
			},
		},
		Actor:  "https://example.com/users/alice",
		Object: "https://example.com/notes/1",
	}

	var flatActivity Activity
	if err := tabletheory.UnmarshalItem(promotedActivityItem(expectedActivity), &flatActivity); err != nil {
		fmt.Fprintf(os.Stderr, "public-api-contracts: unexpected error unmarshalling flat promoted embed payload\n")
		os.Exit(1)
	}
	if !sameContractActivity(flatActivity, expectedActivity) {
		fmt.Fprintf(os.Stderr, "public-api-contracts: unexpected field values after flat promoted embed unmarshal\n")
		os.Exit(1)
	}

	var legacyActivity Activity
	if err := tabletheory.UnmarshalItem(legacyPromotedActivityItem(expectedActivity), &legacyActivity); err != nil {
		fmt.Fprintf(os.Stderr, "public-api-contracts: unexpected error unmarshalling legacy promoted embed payload\n")
		os.Exit(1)
	}
	if !sameContractActivity(legacyActivity, expectedActivity) {
		fmt.Fprintf(os.Stderr, "public-api-contracts: unexpected field values after legacy promoted embed unmarshal\n")
		os.Exit(1)
	}

	var flatStreamActivity Activity
	if err := tabletheory.UnmarshalStreamImage(promotedActivityStreamImage(expectedActivity), &flatStreamActivity); err != nil {
		fmt.Fprintf(os.Stderr, "public-api-contracts: unexpected error unmarshalling flat promoted embed stream image\n")
		os.Exit(1)
	}
	if !sameContractActivity(flatStreamActivity, expectedActivity) {
		fmt.Fprintf(os.Stderr, "public-api-contracts: unexpected field values after flat promoted embed stream unmarshal\n")
		os.Exit(1)
	}

	var legacyStreamActivity Activity
	if err := tabletheory.UnmarshalStreamImage(legacyPromotedActivityStreamImage(expectedActivity), &legacyStreamActivity); err != nil {
		fmt.Fprintf(os.Stderr, "public-api-contracts: unexpected error unmarshalling legacy promoted embed stream image\n")
		os.Exit(1)
	}
	if !sameContractActivity(legacyStreamActivity, expectedActivity) {
		fmt.Fprintf(os.Stderr, "public-api-contracts: unexpected field values after legacy promoted embed stream unmarshal\n")
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
	err := tabletheory.UnmarshalItem(encryptedItem, &encryptedOut)
	if err == nil || !errors.Is(err, customerrors.ErrEncryptionNotConfigured) {
		fmt.Fprintf(os.Stderr, "public-api-contracts: expected encrypted unmarshal to fail closed\n")
		os.Exit(1)
	}

	fmt.Println("public-api-contracts: ok")
}

func promotedActivityItem(activity Activity) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"id":     &types.AttributeValueMemberS{Value: activity.ID},
		"type":   &types.AttributeValueMemberS{Value: activity.Type},
		"to":     stringListAttributeValue(activity.To),
		"actor":  &types.AttributeValueMemberS{Value: activity.Actor},
		"object": &types.AttributeValueMemberS{Value: activity.Object},
	}
}

func legacyPromotedActivityItem(activity Activity) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"baseObject": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"id":   &types.AttributeValueMemberS{Value: activity.ID},
			"type": &types.AttributeValueMemberS{Value: activity.Type},
			"to":   stringListAttributeValue(activity.To),
		}},
		"actor":  &types.AttributeValueMemberS{Value: activity.Actor},
		"object": &types.AttributeValueMemberS{Value: activity.Object},
	}
}

func promotedActivityStreamImage(activity Activity) map[string]events.DynamoDBAttributeValue {
	return map[string]events.DynamoDBAttributeValue{
		"id":     events.NewStringAttribute(activity.ID),
		"type":   events.NewStringAttribute(activity.Type),
		"to":     stringListStreamAttributeValue(activity.To),
		"actor":  events.NewStringAttribute(activity.Actor),
		"object": events.NewStringAttribute(activity.Object),
	}
}

func legacyPromotedActivityStreamImage(activity Activity) map[string]events.DynamoDBAttributeValue {
	return map[string]events.DynamoDBAttributeValue{
		"baseObject": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"id":   events.NewStringAttribute(activity.ID),
			"type": events.NewStringAttribute(activity.Type),
			"to":   stringListStreamAttributeValue(activity.To),
		}),
		"actor":  events.NewStringAttribute(activity.Actor),
		"object": events.NewStringAttribute(activity.Object),
	}
}

func sameContractActivity(a, b Activity) bool {
	if a.ID != b.ID || a.Type != b.Type || a.Actor != b.Actor || a.Object != b.Object {
		return false
	}
	if len(a.To) != len(b.To) {
		return false
	}
	for i := range a.To {
		if a.To[i] != b.To[i] {
			return false
		}
	}
	return true
}

func stringListAttributeValue(values []string) *types.AttributeValueMemberL {
	items := make([]types.AttributeValue, 0, len(values))
	for _, value := range values {
		items = append(items, &types.AttributeValueMemberS{Value: value})
	}
	return &types.AttributeValueMemberL{Value: items}
}

func stringListStreamAttributeValue(values []string) events.DynamoDBAttributeValue {
	items := make([]events.DynamoDBAttributeValue, 0, len(values))
	for _, value := range values {
		items = append(items, events.NewStringAttribute(value))
	}
	return events.NewListAttribute(items)
}
