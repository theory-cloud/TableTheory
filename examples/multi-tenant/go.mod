module github.com/theory-cloud/tabletheory/examples/multi-tenant

go 1.26

toolchain go1.26.4

require (
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/rs/cors v1.11.1
	github.com/stretchr/testify v1.11.1
	github.com/theory-cloud/tabletheory v1.8.3
	golang.org/x/crypto v0.53.0
)

require (
	github.com/aws/aws-lambda-go v1.54.0 // indirect
	github.com/aws/aws-sdk-go-v2 v1.42.1 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.32.27 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.26 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.59.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.12.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.30 // indirect
	github.com/aws/aws-sdk-go-v2/service/kms v1.53.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.2.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.31.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.36.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.43.5 // indirect
	github.com/aws/smithy-go v1.27.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/theory-cloud/tabletheory => ../..
