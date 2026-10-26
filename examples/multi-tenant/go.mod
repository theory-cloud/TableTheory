module github.com/theory-cloud/tabletheory/v6/examples/multi-tenant

go 1.27

toolchain go1.27.3

require (
	github.com/aws/aws-lambda-go v1.55.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/rs/cors v1.11.1
	github.com/stretchr/testify v1.12.1
	github.com/theory-cloud/tabletheory/v6 v6.0.0
	golang.org/x/crypto v0.58.0
)

require (
	github.com/aws/aws-sdk-go-v2 v1.47.4 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.33.10 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.20.10 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.20.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.7 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.7 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.72.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.13.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/kms v1.61.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.10.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.38.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.43.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.51.5 // indirect
	github.com/aws/smithy-go v1.28.6 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
)

replace github.com/theory-cloud/tabletheory/v6 => ../..
