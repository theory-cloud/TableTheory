package theorydb

import (
	"testing"
	"time"

	"github.com/theory-cloud/tabletheory/pkg/marshal"
	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

// Test model for benchmarking
type BenchUser struct {
	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	Metadata  map[string]string
	ID        string `theorydb:"pk"`
	Email     string `theorydb:"sk"`
	Name      string
	Tags      []string
	Age       int
	Balance   float64
	Version   int64 `theorydb:"version"`
	IsActive  bool
}

func BenchmarkMarshalItem_Current(b *testing.B) {
	// Setup
	db := &DB{
		converter: pkgTypes.NewConverter(),
	}
	marshaler := marshal.New(db.converter)

	metadata := &model.Metadata{
		TableName: "Users",
		Fields: map[string]*model.FieldMetadata{
			"ID": {
				Name:   "ID",
				DBName: "id",
				Index:  0,
				IsPK:   true,
			},
			"Email": {
				Name:   "Email",
				DBName: "email",
				Index:  1,
				IsSK:   true,
			},
			"Name": {
				Name:   "Name",
				DBName: "name",
				Index:  2,
			},
			"Age": {
				Name:   "Age",
				DBName: "age",
				Index:  3,
			},
			"IsActive": {
				Name:   "IsActive",
				DBName: "is_active",
				Index:  4,
			},
			"Balance": {
				Name:   "Balance",
				DBName: "balance",
				Index:  5,
			},
			"Tags": {
				Name:   "Tags",
				DBName: "tags",
				Index:  6,
			},
			"Metadata": {
				Name:   "Metadata",
				DBName: "metadata",
				Index:  7,
			},
			"CreatedAt": {
				Name:        "CreatedAt",
				DBName:      "created_at",
				Index:       8,
				IsCreatedAt: true,
			},
			"UpdatedAt": {
				Name:        "UpdatedAt",
				DBName:      "updated_at",
				Index:       9,
				IsUpdatedAt: true,
			},
			"Version": {
				Name:      "Version",
				DBName:    "version",
				Index:     10,
				IsVersion: true,
			},
		},
	}

	user := &BenchUser{
		ID:       "user123",
		Email:    "test@example.com",
		Name:     "John Doe",
		Age:      30,
		IsActive: true,
		Balance:  100.50,
		Tags:     []string{"premium", "verified"},
		Metadata: map[string]string{
			"source":  "mobile",
			"country": "US",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Version:   1,
	}

	// Reset the timer
	b.ResetTimer()
	b.ReportAllocs()

	// Run benchmark
	for i := 0; i < b.N; i++ {
		_, err := marshaler.MarshalItem(user, metadata)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalItem_SimpleStruct(b *testing.B) {
	// Benchmark with a simpler struct
	type SimpleUser struct {
		ID    string `theorydb:"pk"`
		Email string `theorydb:"sk"`
		Name  string
		Age   int
	}

	db := &DB{
		converter: pkgTypes.NewConverter(),
	}
	marshaler := marshal.New(db.converter)

	metadata := &model.Metadata{
		TableName: "Users",
		Fields: map[string]*model.FieldMetadata{
			"ID": {
				Name:   "ID",
				DBName: "id",
				Index:  0,
				IsPK:   true,
			},
			"Email": {
				Name:   "Email",
				DBName: "email",
				Index:  1,
				IsSK:   true,
			},
			"Name": {
				Name:   "Name",
				DBName: "name",
				Index:  2,
			},
			"Age": {
				Name:   "Age",
				DBName: "age",
				Index:  3,
			},
		},
	}

	user := &SimpleUser{
		ID:    "user123",
		Email: "test@example.com",
		Name:  "John Doe",
		Age:   30,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := marshaler.MarshalItem(user, metadata)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark for comparison with AWS SDK's dynamodbattribute
func BenchmarkMarshalItem_PrimitivesOnly(b *testing.B) {
	type PrimitiveUser struct {
		ID       string `theorydb:"pk"`
		Name     string
		Age      int
		IsActive bool
		Balance  float64
	}

	db := &DB{
		converter: pkgTypes.NewConverter(),
	}
	marshaler := marshal.New(db.converter)

	metadata := &model.Metadata{
		TableName: "Users",
		Fields: map[string]*model.FieldMetadata{
			"ID": {
				Name:   "ID",
				DBName: "id",
				Index:  0,
				IsPK:   true,
			},
			"Name": {
				Name:   "Name",
				DBName: "name",
				Index:  1,
			},
			"Age": {
				Name:   "Age",
				DBName: "age",
				Index:  2,
			},
			"IsActive": {
				Name:   "IsActive",
				DBName: "is_active",
				Index:  3,
			},
			"Balance": {
				Name:   "Balance",
				DBName: "balance",
				Index:  4,
			},
		},
	}

	user := &PrimitiveUser{
		ID:       "user123",
		Name:     "John Doe",
		Age:      30,
		IsActive: true,
		Balance:  100.50,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := marshaler.MarshalItem(user, metadata)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark the optimized marshaler
func BenchmarkMarshalItem_Optimized(b *testing.B) {
	// Import the marshal package
	m := marshal.New(nil)

	metadata := &model.Metadata{
		TableName: "Users",
		Fields: map[string]*model.FieldMetadata{
			"ID": {
				Name:   "ID",
				DBName: "id",
				Index:  0,
				IsPK:   true,
			},
			"Email": {
				Name:   "Email",
				DBName: "email",
				Index:  1,
				IsSK:   true,
			},
			"Name": {
				Name:   "Name",
				DBName: "name",
				Index:  2,
			},
			"Age": {
				Name:   "Age",
				DBName: "age",
				Index:  3,
			},
			"IsActive": {
				Name:   "IsActive",
				DBName: "is_active",
				Index:  4,
			},
			"Balance": {
				Name:   "Balance",
				DBName: "balance",
				Index:  5,
			},
			"Tags": {
				Name:   "Tags",
				DBName: "tags",
				Index:  6,
			},
			"Metadata": {
				Name:   "Metadata",
				DBName: "metadata",
				Index:  7,
			},
			"CreatedAt": {
				Name:        "CreatedAt",
				DBName:      "created_at",
				Index:       8,
				IsCreatedAt: true,
			},
			"UpdatedAt": {
				Name:        "UpdatedAt",
				DBName:      "updated_at",
				Index:       9,
				IsUpdatedAt: true,
			},
			"Version": {
				Name:      "Version",
				DBName:    "version",
				Index:     10,
				IsVersion: true,
			},
		},
	}

	user := &BenchUser{
		ID:       "user123",
		Email:    "test@example.com",
		Name:     "John Doe",
		Age:      30,
		IsActive: true,
		Balance:  100.50,
		Tags:     []string{"premium", "verified"},
		Metadata: map[string]string{
			"source":  "mobile",
			"country": "US",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Version:   1,
	}

	// Warm up the cache
	if _, err := m.MarshalItem(user, metadata); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := m.MarshalItem(user, metadata)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark comparing current vs optimized
func BenchmarkMarshalItem_Comparison(b *testing.B) {
	metadata := &model.Metadata{
		TableName: "Users",
		Fields: map[string]*model.FieldMetadata{
			"ID": {
				Name:   "ID",
				DBName: "id",
				Index:  0,
				IsPK:   true,
			},
			"Email": {
				Name:   "Email",
				DBName: "email",
				Index:  1,
				IsSK:   true,
			},
			"Name": {
				Name:   "Name",
				DBName: "name",
				Index:  2,
			},
			"Age": {
				Name:   "Age",
				DBName: "age",
				Index:  3,
			},
			"IsActive": {
				Name:   "IsActive",
				DBName: "is_active",
				Index:  4,
			},
			"Balance": {
				Name:   "Balance",
				DBName: "balance",
				Index:  5,
			},
		},
	}

	type SimpleUser struct {
		ID       string `theorydb:"pk"`
		Email    string `theorydb:"sk"`
		Name     string
		Age      int
		IsActive bool
		Balance  float64
	}

	user := &SimpleUser{
		ID:       "user123",
		Email:    "test@example.com",
		Name:     "John Doe",
		Age:      30,
		IsActive: true,
		Balance:  100.50,
	}

	b.Run("Current", func(b *testing.B) {
		db := &DB{
			converter: pkgTypes.NewConverter(),
		}
		marshaler := marshal.New(db.converter)

		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := marshaler.MarshalItem(user, metadata)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Optimized", func(b *testing.B) {
		converter := pkgTypes.NewConverter()
		db := &DB{
			converter: converter,
			marshaler: marshal.New(converter),
		}

		// Warm up cache
		if _, err := db.marshaler.MarshalItem(user, metadata); err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := db.marshaler.MarshalItem(user, metadata)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

type BenchmarkModel struct {
	CreatedAt time.Time `theorydb:"attr:createdAt"`
	ID        string    `theorydb:"pk,attr:id"`
	Name      string    `theorydb:"attr:name"`
}

func (b BenchmarkModel) TableName() string {
	return "benchmark_table"
}

func BenchmarkGetItemDirect(b *testing.B) {
	// Setup mock or local DynamoDB
	db, err := NewBasic(session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	if err != nil {
		b.Fatal(err)
	}

	model := &BenchmarkModel{ID: "test-id"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result BenchmarkModel
		if err := db.Model(model).Where("id", "=", "test-id").First(&result); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetItemByAttribute(b *testing.B) {
	// Test querying by DynamoDB attribute name
	db, err := NewBasic(session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result BenchmarkModel
		if err := db.Model(&BenchmarkModel{}).Where("id", "=", "test-id").First(&result); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetItemByGoFieldName tests querying by Go field name
func BenchmarkGetItemByGoFieldName(b *testing.B) {
	db, err := NewBasic(session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result BenchmarkModel
		if err := db.Model(&BenchmarkModel{}).Where("ID", "=", "test-id").First(&result); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetItemWithProjection tests GetItem with field selection
func BenchmarkGetItemWithProjection(b *testing.B) {
	db, err := NewBasic(session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result BenchmarkModel
		if err := db.Model(&BenchmarkModel{}).Where("id", "=", "test-id").Select("Name").First(&result); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMetadataCaching tests the metadata cache effectiveness
func BenchmarkMetadataCaching(b *testing.B) {
	db, err := NewBasic(session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	if err != nil {
		b.Fatal(err)
	}

	// Pre-warm the cache
	_ = db.Model(&BenchmarkModel{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = db.Model(&BenchmarkModel{})
	}
}

// BenchmarkQueryOperation tests Query performance when GetItem isn't used
func BenchmarkQueryOperation(b *testing.B) {
	db, err := NewBasic(session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var results []BenchmarkModel
		if err := db.Model(&BenchmarkModel{}).Where("Name", "=", "test-name").All(&results); err != nil {
			b.Fatal(err)
		}
	}
}
