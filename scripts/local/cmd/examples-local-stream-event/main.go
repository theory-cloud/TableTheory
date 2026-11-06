// Command examples-local-stream-event performs a real DynamoDB write (and, in
// ttl-remove mode, a real delete) and reads the resulting record from the
// table's DynamoDB stream with DescribeStream, GetShardIterator and GetRecords.
//
// It never derives a record from the stored item. The record's event id,
// sequence number, keys, images and stream ARN all come from DynamoDB Local
// Streams. The one local substitution is the documented TTL service identity on
// a REMOVE record: DynamoDB Local does not run the TTL sweeper, so the harness
// deletes the item itself and adds the identity AWS documents for TTL deletes
// (https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/time-to-live-ttl-streams.html).
//
// See scripts/local/internal/streams for the reader, the Lambda event shape and
// the deviations the local backend has.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodbstreams"

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/scripts/local/internal/streams"
)

const (
	dummyAccessKeyID     = "dummy"
	dummySecretAccessKey = "dummy"
	streamRegion         = "us-east-1"
	defaultTable         = "lambda_stream_notes"
)

type streamNote struct {
	PK    string `theorydb:"pk,attr:pk" json:"pk"`
	SK    string `theorydb:"sk,attr:sk" json:"sk"`
	Value int    `json:"value"`
}

// insertTable lets the harness name the table from the command line while the
// model still resolves one name through TableName.
var insertTable = defaultTable

func (streamNote) TableName() string { return insertTable }

type result struct {
	Source           string           `json:"source"`
	Mode             string           `json:"mode"`
	Table            string           `json:"table"`
	StreamARN        string           `json:"stream_arn"`
	StreamLabel      string           `json:"stream_label"`
	Event            streams.Event    `json:"event"`
	Record           *streams.Record  `json:"record,omitempty"`
	Records          []streams.Record `json:"records,omitempty"`
	Insert           *streams.Record  `json:"insert,omitempty"`
	Modify           *streams.Record  `json:"modify,omitempty"`
	Remove           *streams.Record  `json:"remove,omitempty"`
	StoredImage      map[string]any   `json:"stored_image,omitempty"`
	OldTTLValue      any              `json:"old_ttl_value,omitempty"`
	TTLIdentityAdded bool             `json:"ttl_identity_added"`
	EventID          string           `json:"event_id"`
	SequenceNumber   string           `json:"sequence_number"`
	EventName        string           `json:"event_name"`
	EventSourceARN   string           `json:"event_source_arn"`
}

func main() {
	mode := flag.String("mode", "insert", "insert or ttl-remove")
	table := flag.String("table", defaultTable, "table to write and read the stream from")
	pk := flag.String("pk", "NOTE#local", "partition key for the stored item")
	sk := flag.String("sk", "META", "sort key for the stored item")
	value := flag.Int("value", 42, "numeric value for the inserted note")
	ttlAttribute := flag.String("ttl-attribute", "expires_at", "TTL attribute for ttl-remove mode")
	timeout := flag.Duration("timeout", 30*time.Second, "how long to wait for the stream record")
	flag.Parse()

	if err := run(context.Background(), *mode, *table, *pk, *sk, *value, *ttlAttribute, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "examples-local-stream-event: FAIL: %v\n", err)
		os.Exit(1)
	}
}

func run(
	ctx context.Context,
	mode, table, pk, sk string,
	value int,
	ttlAttribute string,
	timeout time.Duration,
) error {
	if err := assertHarnessEnvironment(ctx); err != nil {
		return err
	}
	endpoint, err := localEndpoint()
	if err != nil {
		return err
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	ddb := dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) {
		options.BaseEndpoint = aws.String(endpoint)
	})
	streamClient := dynamodbstreams.NewFromConfig(cfg, func(options *dynamodbstreams.Options) {
		options.BaseEndpoint = aws.String(endpoint)
	})

	switch mode {
	case "insert":
		return insert(ctx, ddb, streamClient, table, pk, sk, value, timeout)
	case "ttl-remove":
		return ttlRemove(ctx, ddb, streamClient, table, pk, sk, ttlAttribute, timeout)
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}
}

func insert(
	ctx context.Context,
	ddb *dynamodb.Client,
	streamClient *dynamodbstreams.Client,
	table, pk, sk string,
	value int,
	timeout time.Duration,
) error {
	insertTable = table
	db, err := tabletheory.New(tabletheory.Config{Region: streamRegion})
	if err != nil {
		return err
	}
	if err := db.EnsureTable(&streamNote{}); err != nil {
		return fmt.Errorf("ensure %s: %w", table, err)
	}
	metadata, err := streams.Ensure(ctx, ddb, table, timeout)
	if err != nil {
		return err
	}
	note := &streamNote{PK: pk, SK: sk, Value: value}
	if err := db.Model(note).CreateOrUpdate(); err != nil {
		return fmt.Errorf("write note: %w", err)
	}

	_, record, err := streams.Read(ctx, streamClient, metadata.StreamARN, func(record streams.Record) bool {
		return record.EventName == "INSERT" && matchesKey(record, "pk", "sk", pk, sk)
	}, timeout)
	if err != nil {
		return err
	}

	emit(result{
		Source:         "dynamodb-streams",
		Mode:           "insert",
		Table:          metadata.Table,
		StreamARN:      metadata.StreamARN,
		StreamLabel:    metadata.StreamLabel,
		Event:          streams.EventFor(record),
		Record:         &record,
		EventID:        record.EventID,
		SequenceNumber: record.DynamoDB.SequenceNumber,
		EventName:      record.EventName,
		EventSourceARN: record.EventSourceARN,
	})
	return nil
}

func ttlRemove(
	ctx context.Context,
	ddb *dynamodb.Client,
	streamClient *dynamodbstreams.Client,
	table, pk, sk, ttlAttribute string,
	timeout time.Duration,
) error {
	metadata, err := streams.Ensure(ctx, ddb, table, timeout)
	if err != nil {
		return err
	}
	key := map[string]ddbtypes.AttributeValue{
		"pk": &ddbtypes.AttributeValueMemberS{Value: pk},
		"sk": &ddbtypes.AttributeValueMemberS{Value: sk},
	}
	expired := time.Now().Add(-time.Hour).Unix()
	put := func(payload string) error {
		_, err := ddb.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(metadata.Table),
			Item: map[string]ddbtypes.AttributeValue{
				"pk":         &ddbtypes.AttributeValueMemberS{Value: pk},
				"sk":         &ddbtypes.AttributeValueMemberS{Value: sk},
				"payload":    &ddbtypes.AttributeValueMemberS{Value: payload},
				ttlAttribute: &ddbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", expired)},
			},
		})
		if err != nil {
			return fmt.Errorf("write %s/%s: %w", pk, sk, err)
		}
		return nil
	}

	// An INSERT followed by a MODIFY gives the row a real non-TTL record to
	// assert the handler's skip path against; the DELETE produces the REMOVE.
	if err := put("expired evidence"); err != nil {
		return err
	}
	if err := put("expired evidence v2"); err != nil {
		return err
	}

	stored, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(metadata.Table),
		Key:            key,
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("read back %s/%s: %w", pk, sk, err)
	}
	if len(stored.Item) == 0 {
		return fmt.Errorf("written item %s/%s could not be read back", pk, sk)
	}
	storedImage, err := imageJSON(stored.Item)
	if err != nil {
		return err
	}

	if _, err := ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(metadata.Table),
		Key:       key,
	}); err != nil {
		return fmt.Errorf("delete %s/%s: %w", pk, sk, err)
	}

	collected, remove, err := streams.Read(ctx, streamClient, metadata.StreamARN, func(record streams.Record) bool {
		return record.EventName == "REMOVE" && matchesKey(record, "pk", "sk", pk, sk)
	}, timeout)
	if err != nil {
		return err
	}
	if len(remove.DynamoDB.OldImage) == 0 {
		return fmt.Errorf("REMOVE record has no OldImage")
	}
	// The OldImage must be the item DynamoDB stored, not an image derived from
	// the put request.
	if !reflect.DeepEqual(storedImage, remove.DynamoDB.OldImage) {
		return fmt.Errorf(
			"REMOVE OldImage does not match the stored item:\nstored: %v\nrecord: %v",
			storedImage, remove.DynamoDB.OldImage,
		)
	}

	var inserted, modified *streams.Record
	for index := range collected {
		record := collected[index]
		if !matchesKey(record, "pk", "sk", pk, sk) {
			continue
		}
		switch record.EventName {
		case "INSERT":
			inserted = &record
		case "MODIFY":
			modified = &record
		}
	}
	if inserted == nil {
		return fmt.Errorf("the stream read did not include the INSERT record")
	}
	if modified == nil {
		return fmt.Errorf("the stream read did not include the MODIFY record")
	}

	oldTTLValue := remove.DynamoDB.OldImage[ttlAttribute]
	if oldTTLValue == nil {
		return fmt.Errorf("REMOVE OldImage has no %s attribute", ttlAttribute)
	}
	identityAdded := streams.MarkTTLExpiry(&remove)

	emit(result{
		Source:           "dynamodb-streams",
		Mode:             "ttl-remove",
		Table:            metadata.Table,
		StreamARN:        metadata.StreamARN,
		StreamLabel:      metadata.StreamLabel,
		Event:            streams.EventFor(remove),
		Records:          collected,
		Insert:           inserted,
		Modify:           modified,
		Remove:           &remove,
		StoredImage:      storedImage,
		OldTTLValue:      oldTTLValue,
		TTLIdentityAdded: identityAdded,
		EventID:          remove.EventID,
		SequenceNumber:   remove.DynamoDB.SequenceNumber,
		EventName:        remove.EventName,
		EventSourceARN:   remove.EventSourceARN,
	})
	return nil
}

func matchesKey(record streams.Record, pkName, skName, pk, sk string) bool {
	return stringAttribute(record.DynamoDB.Keys, pkName) == pk &&
		stringAttribute(record.DynamoDB.Keys, skName) == sk
}

func stringAttribute(image map[string]any, name string) string {
	value, _ := image[name].(map[string]string)
	return value["S"]
}

func imageJSON(image map[string]ddbtypes.AttributeValue) (map[string]any, error) {
	encoded := make(map[string]any, len(image))
	for name, attribute := range image {
		value, err := attributeJSON(attribute)
		if err != nil {
			return nil, fmt.Errorf("attribute %s: %w", name, err)
		}
		encoded[name] = value
	}
	return encoded, nil
}

func attributeJSON(attribute ddbtypes.AttributeValue) (any, error) {
	switch value := attribute.(type) {
	case *ddbtypes.AttributeValueMemberS:
		return map[string]string{"S": value.Value}, nil
	case *ddbtypes.AttributeValueMemberN:
		return map[string]string{"N": value.Value}, nil
	case *ddbtypes.AttributeValueMemberB:
		return map[string]string{"B": base64.StdEncoding.EncodeToString(value.Value)}, nil
	case *ddbtypes.AttributeValueMemberBOOL:
		return map[string]bool{"BOOL": value.Value}, nil
	case *ddbtypes.AttributeValueMemberNULL:
		return map[string]bool{"NULL": value.Value}, nil
	case *ddbtypes.AttributeValueMemberSS:
		return map[string][]string{"SS": value.Value}, nil
	case *ddbtypes.AttributeValueMemberNS:
		return map[string][]string{"NS": value.Value}, nil
	case *ddbtypes.AttributeValueMemberM:
		encoded := make(map[string]any, len(value.Value))
		for name, nested := range value.Value {
			item, err := attributeJSON(nested)
			if err != nil {
				return nil, err
			}
			encoded[name] = item
		}
		return map[string]any{"M": encoded}, nil
	case *ddbtypes.AttributeValueMemberL:
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

func emit(payload result) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "examples-local-stream-event: FAIL: encode result: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(encoded))
}

func localEndpoint() (string, error) {
	for _, name := range []string{"AWS_ENDPOINT_URL_DYNAMODB", "DYNAMODB_ENDPOINT", "AWS_ENDPOINT_URL"} {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			continue
		}
		host := strings.TrimPrefix(strings.TrimPrefix(value, "http://"), "https://")
		if index := strings.IndexAny(host, ":/"); index >= 0 {
			host = host[:index]
		}
		switch strings.ToLower(host) {
		case "127.0.0.1", "::1", "localhost":
			return value, nil
		default:
			return "", fmt.Errorf("%s host %q is not loopback; refusing to run", name, host)
		}
	}
	return "", fmt.Errorf("no DynamoDB endpoint is set; the harness must own the endpoint")
}

func assertHarnessEnvironment(ctx context.Context) error {
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
	return nil
}
