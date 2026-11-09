module github.com/theory-cloud/tabletheory/v6

go 1.27

toolchain go1.27.3

require (
	github.com/aws/aws-lambda-go v1.55.1
	github.com/aws/aws-sdk-go-v2 v1.47.7
	github.com/aws/aws-sdk-go-v2/config v1.33.15
	github.com/aws/aws-sdk-go-v2/credentials v1.20.15
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.73.1
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.46.6
	github.com/aws/aws-sdk-go-v2/service/kms v1.62.4
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.8
	github.com/aws/aws-sdk-go-v2/service/sqs v1.52.10
	github.com/aws/aws-sdk-go-v2/service/sts v1.51.8
	github.com/aws/smithy-go v1.28.8
	github.com/google/uuid v1.6.0
	github.com/stretchr/testify v1.12.1
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/kr/pty v1.1.1 => github.com/kr/pty v1.1.8

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.26 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.20.7 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.13.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.12.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.39.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.44.3 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/stretchr/objx v0.8.0 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)
