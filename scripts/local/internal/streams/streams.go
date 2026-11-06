// Package streams reads DynamoDB Streams from the local profile.
//
// DynamoDB Local implements the DynamoDB Streams API: a table created with a
// stream specification (or updated with UpdateTable) exposes
// DescribeTable.LatestStreamArn, and DescribeStream, GetShardIterator and
// GetRecords return the records of real writes. The harness therefore reads the
// stream AWS publishes rather than deriving a record from the stored item.
//
// The record JSON this package emits is the Lambda stream-event shape:
// https://docs.aws.amazon.com/lambda/latest/dg/with-ddb.html
//
// Two differences from a deployed event-source mapping are handled here and
// named in docs/examples/local-harness.md:
//
//   - DynamoDB Streams records do not carry eventSourceARN; Lambda adds the
//     stream ARN when it invokes the function. The reader fills it from the
//     stream ARN DescribeTable/DescribeStream reports.
//   - DynamoDB Local reports the stream ARN in the ddblocal region and does not
//     run the TTL sweeper, so a delete it performs is not attributed to the TTL
//     service. MarkTTLExpiry adds the documented Service identity to a REMOVE
//     record the harness produced with a real DeleteItem. The identity shape is
//     documented at
//     https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/time-to-live-ttl-streams.html
package streams

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodbstreams"
	streamtypes "github.com/aws/aws-sdk-go-v2/service/dynamodbstreams/types"
)

const pollInterval = 100 * time.Millisecond

// Metadata identifies the stream a table exposes.
type Metadata struct {
	Table       string
	StreamARN   string
	StreamLabel string
}

// Event is the Lambda stream-event envelope.
type Event struct {
	Records []Record `json:"Records"`
}

// Record is one DynamoDB stream record in the Lambda event shape.
type Record struct {
	EventID        string    `json:"eventID"`
	EventName      string    `json:"eventName"`
	EventVersion   string    `json:"eventVersion"`
	EventSource    string    `json:"eventSource"`
	AWSRegion      string    `json:"awsRegion"`
	EventSourceARN string    `json:"eventSourceARN"`
	UserIdentity   *Identity `json:"userIdentity,omitempty"`
	DynamoDB       Stream    `json:"dynamodb"`
}

// Identity is the record's userIdentity block.
type Identity struct {
	Type        string `json:"type"`
	PrincipalID string `json:"principalId"`
}

// Stream is the record's dynamodb block. The fields are ordered so the
// pointer-bearing values come first; the JSON shape is unchanged.
type Stream struct {
	Keys                        map[string]any `json:"Keys"`
	NewImage                    map[string]any `json:"NewImage,omitempty"`
	OldImage                    map[string]any `json:"OldImage,omitempty"`
	SequenceNumber              string         `json:"SequenceNumber"`
	StreamViewType              string         `json:"StreamViewType"`
	ApproximateCreationDateTime float64        `json:"ApproximateCreationDateTime,omitempty"`
	SizeBytes                   int64          `json:"SizeBytes"`
}

// Ensure enables a NEW_AND_OLD_IMAGES stream on an existing table and returns
// the stream ARN. It is idempotent: a table that already has a stream is not
// updated again, so repeated harness rows reuse the same stream.
func Ensure(
	ctx context.Context,
	client *dynamodb.Client,
	table string,
	timeout time.Duration,
) (Metadata, error) {
	metadata, err := describeStream(ctx, client, table)
	if err != nil {
		return Metadata{}, err
	}
	if metadata.StreamARN != "" {
		return metadata, nil
	}

	if _, err = client.UpdateTable(ctx, &dynamodb.UpdateTableInput{
		TableName: aws.String(table),
		StreamSpecification: &ddbtypes.StreamSpecification{
			StreamEnabled:  aws.Bool(true),
			StreamViewType: ddbtypes.StreamViewTypeNewAndOldImages,
		},
	}); err != nil {
		return Metadata{}, fmt.Errorf("enable stream on %s: %w", table, err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		metadata, err = describeStream(ctx, client, table)
		if err != nil {
			return Metadata{}, err
		}
		if metadata.StreamARN != "" {
			return metadata, nil
		}
		time.Sleep(pollInterval)
	}
	return Metadata{}, fmt.Errorf("stream on %s was not enabled within %s", table, timeout)
}

func describeStream(
	ctx context.Context,
	client *dynamodb.Client,
	table string,
) (Metadata, error) {
	out, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(table),
	})
	if err != nil {
		return Metadata{}, fmt.Errorf("describe %s: %w", table, err)
	}
	if out.Table == nil {
		return Metadata{}, fmt.Errorf("describe %s returned no table", table)
	}
	return Metadata{
		Table:       table,
		StreamARN:   aws.ToString(out.Table.LatestStreamArn),
		StreamLabel: aws.ToString(out.Table.LatestStreamLabel),
	}, nil
}

// Read polls the stream from TRIM_HORIZON until match accepts one of the
// records read so far, or the deadline expires. It returns every record it
// read, the matched record, and an error when no match arrived.
func Read(
	ctx context.Context,
	client *dynamodbstreams.Client,
	streamARN string,
	match func(Record) bool,
	timeout time.Duration,
) ([]Record, Record, error) {
	description, err := client.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
		StreamArn: aws.String(streamARN),
	})
	if err != nil {
		return nil, Record{}, fmt.Errorf("describe stream %s: %w", streamARN, err)
	}
	if description.StreamDescription == nil {
		return nil, Record{}, fmt.Errorf("describe stream %s returned no description", streamARN)
	}

	deadline := time.Now().Add(timeout)
	var collected []Record
	for _, shard := range description.StreamDescription.Shards {
		if shard.ShardId == nil {
			continue
		}
		iterator, err := client.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
			StreamArn:         aws.String(streamARN),
			ShardId:           shard.ShardId,
			ShardIteratorType: streamtypes.ShardIteratorTypeTrimHorizon,
		})
		if err != nil {
			return collected, Record{}, fmt.Errorf("shard iterator for %s: %w", aws.ToString(shard.ShardId), err)
		}
		for iterator.ShardIterator != nil && time.Now().Before(deadline) {
			out, err := client.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{
				ShardIterator: iterator.ShardIterator,
				Limit:         aws.Int32(1000),
			})
			if err != nil {
				return collected, Record{}, fmt.Errorf("get records from %s: %w", aws.ToString(shard.ShardId), err)
			}
			for _, raw := range out.Records {
				record, err := convertRecord(raw, streamARN)
				if err != nil {
					return collected, Record{}, err
				}
				collected = append(collected, record)
				if match(record) {
					return collected, record, nil
				}
			}
			iterator.ShardIterator = out.NextShardIterator
			if len(out.Records) == 0 {
				time.Sleep(pollInterval)
			}
		}
	}
	return collected, Record{}, fmt.Errorf("no matching stream record arrived within %s", timeout)
}

// MarkTTLExpiry adds the documented DynamoDB TTL service identity to a REMOVE
// record the harness produced with a real delete. It reports whether it added
// the identity, so a caller can report that the TTL attribution is the one
// local substitution. DynamoDB Local does not run the TTL sweeper; the record's
// keys, images, event id, sequence number and stream ARN are the stream's.
func MarkTTLExpiry(record *Record) bool {
	if record == nil || record.EventName != "REMOVE" || record.UserIdentity != nil {
		return false
	}
	record.UserIdentity = &Identity{
		Type:        "Service",
		PrincipalID: "dynamodb.amazonaws.com",
	}
	return true
}

// EventFor wraps records in the Lambda event envelope.
func EventFor(records ...Record) Event {
	return Event{Records: records}
}

func convertRecord(raw streamtypes.Record, streamARN string) (Record, error) {
	record := Record{
		EventID:        aws.ToString(raw.EventID),
		EventName:      string(raw.EventName),
		EventVersion:   aws.ToString(raw.EventVersion),
		EventSource:    aws.ToString(raw.EventSource),
		AWSRegion:      aws.ToString(raw.AwsRegion),
		EventSourceARN: streamARN,
	}
	if raw.UserIdentity != nil {
		record.UserIdentity = &Identity{
			Type:        aws.ToString(raw.UserIdentity.Type),
			PrincipalID: aws.ToString(raw.UserIdentity.PrincipalId),
		}
	}
	if raw.Dynamodb == nil {
		return record, nil
	}

	keys, err := imageJSON(raw.Dynamodb.Keys)
	if err != nil {
		return Record{}, fmt.Errorf("encode stream keys: %w", err)
	}
	record.DynamoDB = Stream{
		ApproximateCreationDateTime: unixSeconds(raw.Dynamodb.ApproximateCreationDateTime),
		Keys:                        keys,
		SequenceNumber:              aws.ToString(raw.Dynamodb.SequenceNumber),
		SizeBytes:                   aws.ToInt64(raw.Dynamodb.SizeBytes),
		StreamViewType:              string(raw.Dynamodb.StreamViewType),
	}
	if raw.Dynamodb.NewImage != nil {
		record.DynamoDB.NewImage, err = imageJSON(raw.Dynamodb.NewImage)
		if err != nil {
			return Record{}, fmt.Errorf("encode stream NewImage: %w", err)
		}
	}
	if raw.Dynamodb.OldImage != nil {
		record.DynamoDB.OldImage, err = imageJSON(raw.Dynamodb.OldImage)
		if err != nil {
			return Record{}, fmt.Errorf("encode stream OldImage: %w", err)
		}
	}
	return record, nil
}

func unixSeconds(value *time.Time) float64 {
	if value == nil {
		return 0
	}
	return float64(value.UnixNano()) / float64(time.Second)
}

func imageJSON(image map[string]streamtypes.AttributeValue) (map[string]any, error) {
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

// attributeJSON converts a DynamoDB Streams attribute value into the DynamoDB
// JSON a Lambda stream event carries.
func attributeJSON(attribute streamtypes.AttributeValue) (any, error) {
	switch value := attribute.(type) {
	case *streamtypes.AttributeValueMemberS:
		return map[string]string{"S": value.Value}, nil
	case *streamtypes.AttributeValueMemberN:
		return map[string]string{"N": value.Value}, nil
	case *streamtypes.AttributeValueMemberB:
		return map[string]string{"B": base64.StdEncoding.EncodeToString(value.Value)}, nil
	case *streamtypes.AttributeValueMemberBOOL:
		return map[string]bool{"BOOL": value.Value}, nil
	case *streamtypes.AttributeValueMemberNULL:
		return map[string]bool{"NULL": value.Value}, nil
	case *streamtypes.AttributeValueMemberSS:
		return map[string][]string{"SS": value.Value}, nil
	case *streamtypes.AttributeValueMemberNS:
		return map[string][]string{"NS": value.Value}, nil
	case *streamtypes.AttributeValueMemberBS:
		return map[string][]string{"BS": binarySet(value.Value)}, nil
	case *streamtypes.AttributeValueMemberM:
		return mapAttribute(value.Value)
	case *streamtypes.AttributeValueMemberL:
		return listAttribute(value.Value)
	default:
		return nil, fmt.Errorf("unsupported stream attribute type %T", attribute)
	}
}

func binarySet(values [][]byte) []string {
	encoded := make([]string, 0, len(values))
	for _, entry := range values {
		encoded = append(encoded, base64.StdEncoding.EncodeToString(entry))
	}
	return encoded
}

func mapAttribute(values map[string]streamtypes.AttributeValue) (any, error) {
	encoded := make(map[string]any, len(values))
	for name, nested := range values {
		item, err := attributeJSON(nested)
		if err != nil {
			return nil, err
		}
		encoded[name] = item
	}
	return map[string]any{"M": encoded}, nil
}

func listAttribute(values []streamtypes.AttributeValue) (any, error) {
	encoded := make([]any, 0, len(values))
	for _, nested := range values {
		item, err := attributeJSON(nested)
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, item)
	}
	return map[string]any{"L": encoded}, nil
}
