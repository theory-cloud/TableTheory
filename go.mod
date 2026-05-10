module github.com/theory-cloud/tabletheory

go 1.26

toolchain go1.26.3

require (
	github.com/aws/aws-lambda-go v1.53.0
	github.com/aws/aws-sdk-go-v2 v1.41.5
	github.com/aws/aws-sdk-go-v2/config v1.32.11
	github.com/aws/aws-sdk-go-v2/credentials v1.19.11
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.56.1
	github.com/aws/aws-sdk-go-v2/service/kms v1.50.2
	github.com/aws/aws-sdk-go-v2/service/s3 v1.97.3
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.8
	github.com/google/uuid v1.6.0
	github.com/stretchr/testify v1.11.1
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/kr/pty v1.1.1 => github.com/kr/pty v1.1.8

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.8 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.19 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.5 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.11.19 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.16 // indirect
	github.com/aws/smithy-go v1.24.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)
