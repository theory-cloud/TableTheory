package examples

import (
	"os"
	"time"
)

// FaceTheoryCacheMetadata is a minimal metadata row shape for ISR cache tables.
//
// See: docs/facetheory/isr-cache-schema.md
type FaceTheoryCacheMetadata struct {
	_ struct{} `theorydb:"naming:snake_case"`

	PK string `theorydb:"pk,attr:pk" json:"pk"`
	SK string `theorydb:"sk,attr:sk" json:"sk"`

	S3Key             string `theorydb:"attr:s3_key" json:"s3_key"`
	GeneratedAt       int64  `theorydb:"attr:generated_at" json:"generated_at"`
	RevalidateSeconds int64  `theorydb:"attr:revalidate_seconds" json:"revalidate_seconds"`
	ETag              string `theorydb:"attr:etag,omitempty" json:"etag,omitempty"`

	TTL int64 `theorydb:"ttl,attr:ttl,omitempty" json:"ttl,omitempty"`
}

func (FaceTheoryCacheMetadata) TableName() string {
	return os.Getenv("FACETHEORY_CACHE_TABLE_NAME")
}

// FaceTheoryFreshUntilUnix returns the freshness boundary for ISR (not DynamoDB TTL).
func FaceTheoryFreshUntilUnix(meta FaceTheoryCacheMetadata) int64 {
	return meta.GeneratedAt + meta.RevalidateSeconds
}

// FaceTheoryIsFresh reports whether the metadata is within its ISR freshness window.
func FaceTheoryIsFresh(meta FaceTheoryCacheMetadata, now time.Time) bool {
	return now.Unix() < FaceTheoryFreshUntilUnix(meta)
}

// FaceTheoryTTLUnix returns a TTL epoch-seconds value suitable for DynamoDB TTL.
// This is a garbage-collection horizon, not a freshness boundary.
func FaceTheoryTTLUnix(generatedAt int64, retention time.Duration) int64 {
	return generatedAt + int64(retention.Seconds())
}
