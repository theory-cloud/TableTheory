// Command examples-local-stream-event performs one real DynamoDB write through
// TableTheory, reads the stored item back with GetItem, and prints a DynamoDB
// stream record built from that stored image.
//
// DynamoDB Local has no Streams, so the harness feeds this generated record to
// the Python stream handler as a real Lambda invocation. The record is derived
// from the item DynamoDB Local actually stored; the delivery is the named cloud
// gap ("managed stream delivery").
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v6"
)

const (
	dummyAccessKeyID     = "dummy"
	dummySecretAccessKey = "dummy"
	streamTable          = "lambda_stream_notes"
	streamRegion         = "us-east-1"
)

type streamNote struct {
	PK    string `theorydb:"pk,attr:pk" json:"pk"`
	SK    string `theorydb:"sk,attr:sk" json:"sk"`
	Value int    `json:"value"`
}

func (streamNote) TableName() string { return streamTable }

type streamEvent struct {
	Records []streamRecord `json:"Records"`
}

type streamRecord struct {
	EventID        string         `json:"eventID"`
	EventName      string         `json:"eventName"`
	EventVersion   string         `json:"eventVersion"`
	EventSource    string         `json:"eventSource"`
	AWSRegion      string         `json:"awsRegion"`
	EventSourceARN string         `json:"eventSourceARN"`
	DynamoDB       streamDynamoDB `json:"dynamodb"`
}

type streamDynamoDB struct {
	Keys           map[string]any `json:"Keys"`
	NewImage       map[string]any `json:"NewImage"`
	SequenceNumber string         `json:"SequenceNumber"`
	SizeBytes      int            `json:"SizeBytes"`
	StreamViewType string         `json:"StreamViewType"`
}

func main() {
	pk := flag.String("pk", "NOTE#local", "partition key for the stored note")
	sk := flag.String("sk", "META", "sort key for the stored note")
	value := flag.Int("value", 42, "numeric value for the stored note")
	flag.Parse()

	if err := run(context.Background(), *pk, *sk, *value); err != nil {
		fmt.Fprintf(os.Stderr, "examples-local-stream-event: FAIL: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, pk, sk string, value int) error {
	if err := assertHarnessCredentials(ctx); err != nil {
		return err
	}

	db, err := tabletheory.New(tabletheory.Config{Region: streamRegion})
	if err != nil {
		return err
	}
	if err := db.EnsureTable(&streamNote{}); err != nil {
		return fmt.Errorf("ensure %s: %w", streamTable, err)
	}

	note := &streamNote{PK: pk, SK: sk, Value: value}
	if err := db.Model(note).CreateOrUpdate(); err != nil {
		return fmt.Errorf("write note: %w", err)
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	client := dynamodb.NewFromConfig(cfg)
	stored, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(streamTable),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: pk},
			"sk": &types.AttributeValueMemberS{Value: sk},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("read note back: %w", err)
	}
	if len(stored.Item) == 0 {
		return fmt.Errorf("the write did not produce a stored item")
	}

	image := make(map[string]any, len(stored.Item))
	for name, attribute := range stored.Item {
		encoded, err := attributeJSON(attribute)
		if err != nil {
			return fmt.Errorf("encode attribute %s: %w", name, err)
		}
		image[name] = encoded
	}

	keys := map[string]any{}
	for _, key := range []string{"pk", "sk"} {
		encoded, ok := image[key]
		if !ok {
			return fmt.Errorf("stored item has no %s key attribute", key)
		}
		keys[key] = encoded
	}

	size, err := json.Marshal(image)
	if err != nil {
		return fmt.Errorf("marshal image: %w", err)
	}

	event := streamEvent{Records: []streamRecord{{
		EventID:        fmt.Sprintf("local-%d", time.Now().UnixNano()),
		EventName:      "INSERT",
		EventVersion:   "1.1",
		EventSource:    "aws:dynamodb",
		AWSRegion:      streamRegion,
		EventSourceARN: "arn:aws:dynamodb:" + streamRegion + ":123456789012:table/" + streamTable + "/stream/local",
		DynamoDB: streamDynamoDB{
			Keys:           keys,
			NewImage:       image,
			SequenceNumber: fmt.Sprintf("%d", time.Now().UnixNano()),
			SizeBytes:      len(size),
			StreamViewType: "NEW_AND_OLD_IMAGES",
		},
	}}}

	out, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal stream event: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

func attributeJSON(attribute types.AttributeValue) (any, error) {
	switch value := attribute.(type) {
	case *types.AttributeValueMemberS:
		return map[string]string{"S": value.Value}, nil
	case *types.AttributeValueMemberN:
		return map[string]string{"N": value.Value}, nil
	case *types.AttributeValueMemberBOOL:
		return map[string]bool{"BOOL": value.Value}, nil
	case *types.AttributeValueMemberNULL:
		return map[string]bool{"NULL": value.Value}, nil
	case *types.AttributeValueMemberB:
		return map[string][]byte{"B": value.Value}, nil
	case *types.AttributeValueMemberSS:
		return map[string][]string{"SS": value.Value}, nil
	case *types.AttributeValueMemberNS:
		return map[string][]string{"NS": value.Value}, nil
	case *types.AttributeValueMemberM:
		encoded := make(map[string]any, len(value.Value))
		for name, nested := range value.Value {
			item, err := attributeJSON(nested)
			if err != nil {
				return nil, err
			}
			encoded[name] = item
		}
		return map[string]any{"M": encoded}, nil
	case *types.AttributeValueMemberL:
		encoded := make([]any, 0, len(value.Value))
		for _, nested := range value.Value {
			item, err := attributeJSON(nested)
			if err != nil {
				return nil, err
			}
			encoded = append(encoded, item)
		}
		return map[string]any{"L": encoded}, nil
	default:
		return nil, fmt.Errorf("unsupported attribute type %T", attribute)
	}
}

func assertHarnessCredentials(ctx context.Context) error {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("resolve AWS credentials: %w", err)
	}
	if creds.AccessKeyID != dummyAccessKeyID || creds.SecretAccessKey != dummySecretAccessKey {
		return fmt.Errorf("resolved credentials are not the harness dummy static keys; refusing to run")
	}
	if os.Getenv("AWS_ENDPOINT_URL_DYNAMODB") == "" {
		return fmt.Errorf("AWS_ENDPOINT_URL_DYNAMODB is unset; the harness must own the endpoint")
	}
	return nil
}
