module github.com/theory-cloud/tabletheory/v2/examples/basic/notes

go 1.26

toolchain go1.26.5

require (
	github.com/aws/aws-sdk-go-v2/config v1.32.29
	github.com/aws/aws-sdk-go-v2/credentials v1.19.28
	github.com/google/uuid v1.6.0
	github.com/theory-cloud/tabletheory/v2 v2.0.2
)

require (
	github.com/aws/aws-lambda-go v1.54.0 // indirect
	github.com/aws/aws-sdk-go-v2 v1.42.1 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.59.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.12.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.30 // indirect
	github.com/aws/aws-sdk-go-v2/service/kms v1.53.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.4.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.32.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.37.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.44.0 // indirect
	github.com/aws/smithy-go v1.27.3 // indirect
)

replace github.com/theory-cloud/tabletheory/v2 => ../../..
