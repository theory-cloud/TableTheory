# TableTheory Example: Multi-tenant SaaS Platform

## Overview

This example demonstrates how to build a scalable multi-tenant SaaS platform using TableTheory with AWS Lambda. It showcases enterprise-grade patterns for tenant isolation, resource management, and audit logging in a serverless architecture.

## Key Features

- **Tenant Isolation**: Complete data isolation using composite keys
- **User Management**: Cross-organization users with role-based access
- **Project Management**: Resource allocation and team collaboration
- **Usage Tracking**: Real-time resource consumption monitoring
- **Audit Logging**: Compliance-ready audit trails with TTL
- **API Key Management**: Secure programmatic access

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   SaaS App  │────▶│ API Gateway  │────▶│   Lambda    │
└─────────────┘     └──────────────┘     └──────┬──────┘
                                                 │
                                          ┌──────▼──────┐
                                          │  DynamoDB   │
                                          │             │
                                          │ Single Table │
                                          │   Design    │
                                          └─────────────┘

Tenant Isolation Pattern:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
PK: org#123#user#456        SK: METADATA
PK: org#123#project#789     SK: METADATA  
PK: org#123#resource#abc    SK: 2024-01-15T10:00:00Z
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### DynamoDB Schema Design

**Primary Table**
- Composite keys ensure tenant isolation
- All queries require org_id prefix
- No cross-tenant data access possible

**Key Patterns**:
- Users: `PK: org#{orgId}#user#{userId}`
- Projects: `PK: org#{orgId}#project#{projectId}`
- Resources: `PK: org#{orgId}#resource#{resourceId}`
- Audit: `PK: org#{orgId}#{timestamp}#{eventId}`

**Global Secondary Indexes**:
- `gsi-email`: User lookups by email (with org context)
- `gsi-org-projects`: List projects by organization
- `gsi-project-resources`: Track resource usage
- `gsi-user-audit`: User activity tracking

## Quick Start

### Prerequisites
- Go toolchain `go1.27.2`
- AWS CLI configured
- Docker (for local DynamoDB)

### Local Development

1. **Setup**:
```bash
cd examples/multi-tenant
make setup
```

2. **Start services**:
```bash
docker compose up -d
```

3. **Run tests**:
```bash
make test
```

4. **Run the server and authenticate**:
```bash
make run                              # starts the server with a local JWT_SECRET
make token USER=user123 ORG=orgabc    # prints an HS256 bearer token
curl -H "Authorization: Bearer <token>" "http://localhost:8080/organizations/org%23orgabc"
```

Every request is authenticated: the server verifies the bearer token's HS256
signature with `JWT_SECRET` and derives the caller's user and organization from
it. A caller may only reach the organization its token is bound to, so a token
for one organization cannot read or change another organization's resources.

### Authorization model

Authentication only proves who signed the request. Every organization-scoped
route then resolves the caller against the store before a handler reads tenant
data:

- **Active stored principal.** A user route loads the caller's user row in the
  organization its token is bound to and requires `status: active`. Suspended,
  invited, deleted, and missing users receive `403` before any organization,
  user, project, or usage data is read. `GET /organizations` (the tenant
  listing) requires an active stored user too.
- **Allowlisted authorizer context.** The Lambda adapter copies only
  `user_id`, `org_id`, and `api_key_id` from the API Gateway authorizer. A
  request that carries a user identity is evaluated only as that user, so a
  suspended user cannot fall back to an API key on the same request.
- **Directory reads.** Owners and admins may list the organization directory.
  Every other active user may read only their own user record. Members and
  viewers cannot enumerate users.
- **Organization usage.** Organization-wide usage and usage reports stay with
  the existing owner/admin authority. Project-scoped usage keeps the existing
  project rule: owners/admins, assigned users, and an API key bound to that
  project.
- **API-key identity.** `POST /organizations/{org_id}/resources` is an API-key
  route. The key must still be active, unexpired, bound to the URL
  organization, and — if project-scoped — bound to the requested project. A
  request without `api_key_id` is refused with `403`; a wrong or missing
  project is `404`. Neither path creates a usage row.

### Warm failure history versus persistent limits

API-key verification has two different state boundaries:

- **Warm execution environment (process-local).** The verifier bounds how many
  bcrypt comparisons run at once and tracks recent wrong-secret failures per
  key identifier. Five failures inside the window cause later attempts to be
  rejected before another bcrypt comparison. The Lambda authorizer caches one
  handler alongside its database handle, so this history and the bcrypt slots
  survive warm invocations. A cold start, a new deployment, or a parallel
  execution environment starts with empty history; that is a deliberate
  process-local bound, not a distributed lock.
- **Persistent (DynamoDB-backed).** The per-key hourly rate limit counts
  recorded usage in DynamoDB and remains authoritative across cold starts and
  parallel environments. The warm failure history narrows pre-authentication
  CPU work; it does not replace the persistent rate limit.

Successful verifications are never cached. Revocation, rotation, and expiry take
effect on the next record lookup, in every environment.

## API Reference

### Organization Management

#### Create Organization
```http
POST /organizations

{
  "name": "Acme Corp",
  "slug": "acme-corp",
  "owner_email": "admin@acme.com",
  "plan": "starter"
}
```

#### Get Organization
```http
GET /organizations/{org_id}
Authorization: Bearer {token}
```

#### Update Organization Settings
```http
PUT /organizations/{org_id}/settings
Authorization: Bearer {token}

{
  "require_mfa": true,
  "allowed_domains": ["acme.com"],
  "session_timeout": 60
}
```

### User Management

#### Invite User
```http
POST /organizations/{org_id}/invitations
Authorization: Bearer {token}

{
  "email": "user@example.com",
  "role": "member",
  "projects": ["project-1", "project-2"]
}
```

#### List Organization Users
```http
GET /organizations/{org_id}/users
Authorization: Bearer {token}
```

Query Parameters:
- `role`: Filter by role
- `status`: Filter by status (active, invited, suspended)
- `project_id`: Filter by project access

#### Update User Role
```http
PUT /organizations/{org_id}/users/{user_id}
Authorization: Bearer {token}

{
  "role": "admin",
  "projects": ["project-1", "project-2", "project-3"]
}
```

### Project Management

#### Create Project
```http
POST /organizations/{org_id}/projects
Authorization: Bearer {token}

{
  "name": "Mobile App",
  "slug": "mobile-app",
  "type": "mobile",
  "environment": "production",
  "team": [
    {
      "user_id": "user-123",
      "role": "lead"
    }
  ]
}
```

#### List Projects
```http
GET /organizations/{org_id}/projects
Authorization: Bearer {token}
```

#### Get Project with Resource Usage
```http
GET /organizations/{org_id}/projects/{project_id}?include=resources
Authorization: Bearer {token}
```

### Resource Tracking

#### Record Resource Usage
```http
POST /organizations/{org_id}/resources
X-API-Key: {api_key}

{
  "project_id": "project-123",
  "type": "api_call",
  "quantity": 1000,
  "metadata": {
    "endpoint": "/api/v1/process",
    "status_code": "200"
  }
}
```

#### Get Usage Report
```http
GET /organizations/{org_id}/usage?billing_cycle=2024-01
Authorization: Bearer {token}
```

### API Key Management

#### Create API Key
```http
POST /organizations/{org_id}/api-keys
Authorization: Bearer {token}

{
  "name": "Production API Key",
  "project_id": "project-123",
  "scopes": ["read:resources", "write:resources"],
  "rate_limit": 1000
}
```

Response:
```json
{
  "key_id": "key_abc123",
  "key": "sk_live_abcdef123456", // Only shown once
  "key_prefix": "sk_live_"
}
```

### Audit Log

#### Query Audit Log
```http
GET /organizations/{org_id}/audit?start_date=2024-01-01&end_date=2024-01-31
Authorization: Bearer {token}
```

Query Parameters:
- `user_id`: Filter by user
- `action`: Filter by action type
- `resource_type`: Filter by resource type
- `limit`: Results per page

## Deployment

### Environment Variables

```yaml
DYNAMODB_TABLE: saas-platform
JWT_SECRET: your-secret-key
AUDIT_RETENTION_DAYS: 90
DEFAULT_RATE_LIMIT: 1000
```

### Lambda Entrypoint

One binary, `cmd/lambda`, serves every function the SAM template declares. The
`FUNCTION_TYPE` environment variable selects which handler an invocation runs:

| `FUNCTION_TYPE` | Role |
| --- | --- |
| `organization`, `user`, `project`, `resource`, `apikey` | API Gateway proxy handlers, routed through the same table the local server uses (`handlers.NewRouter`, shared with `cmd/local`) |
| `jwt_authorizer`, `apikey_authorizer` | API Gateway custom authorizers, declared as REQUEST authorizers so each receives the path, method, and headers it checks |

Build the entrypoint with `make build`, which produces `bin/bootstrap` and zips
it to `bin/deployment.zip`. The template deploys that binary on the
`provided.al2023` runtime with the `bootstrap` handler.

### AWS SAM Deployment

```bash
sam build
sam deploy --guided
```

### Security Considerations

1. **Tenant Isolation**: Every query includes org_id in composite key
2. **API Authentication**: JWT tokens or API keys, both bound to an organization
3. **Active Principal Authorization**: Every protected route requires an active
   stored user or an active, unexpired API key; directory and organization
   usage reads are limited to owners/admins and self as described above
4. **Two Rate-Limit Boundaries**: Process-local wrong-secret throttling and
   bcrypt slot bounds for a warm environment, plus the persistent
   DynamoDB-backed per-key hourly limit
5. **Audit Trail**: All actions logged with 90-day retention
6. **MFA Support**: Optional two-factor authentication

## Cost Model

### Pricing Tiers

| Plan | Users | Projects | API Calls | Storage | Price |
|------|-------|----------|-----------|---------|-------|
| Free | 3 | 1 | 10K/mo | 1GB | $0 |
| Starter | 10 | 5 | 100K/mo | 10GB | $29/mo |
| Pro | 50 | 20 | 1M/mo | 100GB | $99/mo |
| Enterprise | Unlimited | Unlimited | Custom | Custom | Custom |

### DynamoDB Costs (Estimated)

For 100 organizations with average activity:
- Writes: ~$50/month
- Reads: ~$20/month
- Storage: ~$10/month
- **Total**: ~$80/month

## Key Patterns Demonstrated

### 1. Composite Key Isolation
```go
// All models use org_id in composite primary key
type User struct {
    ID    string `theorydb:"pk,composite:org_id,user_id"`
    OrgID string `theorydb:"extract:org_id"`
    // ...
}
```

### 2. Cross-Organization Users
- Users can belong to multiple organizations
- Email-based lookup with org context
- Separate permissions per organization

### 3. Usage Tracking
- Real-time resource consumption
- Aggregated monthly reports
- Cost allocation by project

### 4. Audit Compliance
- Immutable audit log
- TTL for automatic cleanup
- User activity tracking

### 5. Plan Enforcement
- Resource limits per plan
- Automatic usage capping
- Upgrade prompts

## Performance Optimization

1. **Composite Keys**: Enable efficient tenant queries
2. **GSI Design**: Optimized for common access patterns  
3. **Batch Operations**: Bulk user/project operations
4. **Caching**: Organization settings cached in Lambda
5. **Connection Pooling**: Reuse DynamoDB connections

## Monitoring & Observability

### CloudWatch Metrics
- API latency by organization
- Resource usage by type
- Error rates by tenant
- Billing anomalies

### Alarms
- Excessive API usage
- Failed payments
- Unusual audit activity
- Storage quota exceeded

## Extension Ideas

1. **SSO Integration**: SAML/OAuth support
2. **Webhooks**: Real-time event notifications
3. **Data Export**: Compliance exports
4. **Custom Domains**: Per-organization domains
5. **White-labeling**: Custom branding
6. **Analytics Dashboard**: Usage insights
7. **Terraform Provider**: Infrastructure as code
8. **Mobile SDK**: iOS/Android libraries

## Best Practices

1. **Always Include Org Context**: Never query without org_id
2. **Validate Permissions**: Check user role for every action
3. **Rate Limit Everything**: Prevent abuse
4. **Audit Everything**: Log all state changes
5. **Plan for Scale**: Design for 10,000+ tenants

## Troubleshooting

### Common Issues

1. **Cross-tenant Data Leak**: Check composite key usage
2. **Performance Degradation**: Review GSI hot partitions
3. **Billing Discrepancies**: Verify usage tracking
4. **Permission Errors**: Check role inheritance

### Debug Queries

```go
// List all entities for an organization
db.Model(&models.User{}).
    Where("OrgID", "=", orgID).
    All(&users)

// Verify tenant isolation
db.Model(&models.Project{}).
    Where("ID", "beginsWith", fmt.Sprintf("org#%s#", orgID)).
    All(&projects)
```

## Contributing

This example provides patterns for multi-tenant SaaS. Extensions welcome:
- GraphQL API layer
- Kubernetes operator
- Backup/restore functionality
- Compliance certifications (SOC2, ISO27001)
- Advanced analytics
- Machine learning features 