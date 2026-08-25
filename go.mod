module github.com/theory-cloud/tabletheory/v3

go 1.26

toolchain go1.26.6

require (
	github.com/aws/aws-lambda-go v1.54.0
	github.com/aws/aws-sdk-go-v2 v1.43.7
	github.com/aws/aws-sdk-go-v2/config v1.32.38
	github.com/aws/aws-sdk-go-v2/credentials v1.19.37
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.63.4
	github.com/aws/aws-sdk-go-v2/service/kms v1.55.7
	github.com/aws/aws-sdk-go-v2/service/s3 v1.107.3
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.7
	github.com/aws/smithy-go v1.27.8
	github.com/google/uuid v1.6.0
	github.com/stretchr/testify v1.12.1
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/kr/pty v1.1.1 => github.com/kr/pty v1.1.8

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.18 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.38 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.38 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.38 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.39 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.12.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.38 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.39 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.7 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)
