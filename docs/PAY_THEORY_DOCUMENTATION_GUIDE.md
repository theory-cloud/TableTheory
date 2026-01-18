# Pay Theory Documentation Guide

**Version:** 1.1  
**Last Updated:** January 2026  
**Status:** Official Standard

## Purpose

This guide defines the official documentation convention for all Pay Theory products, services, frameworks, and tools within the Partner Factory repository. It ensures consistency, AI-friendliness, and maintainability across 100+ submodules.

## Quick Reference

### For New Modules
1. Create `docs/` directory
2. Copy standard file structure (see [Template](#standard-file-structure))
3. Create YAML triad (_concepts.yaml, _patterns.yaml, _decisions.yaml)
4. Write core documentation files
5. Add module to parent documentation index

### For Existing Modules
1. Review current docs against checklist (see [Validation](#validation-checklist))
2. Add missing standard files
3. Update YAML files with new features
4. Ensure examples are current and compile
5. Update troubleshooting with recent issues

---

## Table of Contents

- [Documentation Principles](#documentation-principles)
- [Standard File Structure](#standard-file-structure)
- [File Templates](#file-templates)
- [YAML Knowledge Base](#yaml-knowledge-base)
- [Writing Guidelines](#writing-guidelines)
- [Module-Specific Adaptations](#module-specific-adaptations)
- [Development Process Integration](#development-process-integration)
- [Validation Checklist](#validation-checklist)
- [Examples from the Codebase](#examples-from-the-codebase)

---

## Documentation Principles

All Pay Theory documentation follows these six core principles:

### 1. Examples First
Show working code before explaining theory. Every concept must have a concrete, runnable example.

```markdown
❌ BAD:
Lift uses middleware patterns to compose functionality...

✅ GOOD:
```go
// Example: Add logging and auth to your Lambda
app.Use(middleware.Logger())
app.Use(middleware.JWT(jwtConfig))
app.POST("/payments", HandlePayment)
```
```

### 2. Explicit Context
Mark patterns as CORRECT or INCORRECT. Never assume the reader knows which approach is preferred.

```markdown
✅ CORRECT: Use lift.Context for all handlers
❌ INCORRECT: Don't use raw Lambda handlers
```

### 3. Semantic Structure
Use AI training signals and metadata so both humans and AI assistants can learn effectively.

```markdown
<!-- AI Training: This is the documentation index -->
**This directory contains the OFFICIAL documentation...**
```

### 4. Problem-Solution Format
Structure content around problems developers face, not just feature descriptions.

```markdown
### Problem: Rate limiting doesn't work across Lambda instances
**Solution:** Use Limited with DynamoDB backend
```

### 5. Business Value
Explain WHY a feature exists or pattern is preferred, not just HOW to use it.

```markdown
## Why Lift?
- Reduces Lambda boilerplate by 80%
- Prevents common security mistakes
- Provides production-grade observability out of the box
```

### 6. Machine Parsability
Include YAML knowledge bases that enable semantic search and reasoning.

---

## Standard File Structure

Every module with a `docs/` directory must include:

```
module-name/
├── docs/
│   ├── README.md                    # Documentation index (REQUIRED)
│   ├── _concepts.yaml               # Concept hierarchy (REQUIRED)
│   ├── _patterns.yaml               # Usage patterns (REQUIRED)
│   ├── _decisions.yaml              # Decision trees (REQUIRED)
│   ├── getting-started.md           # Installation & first use (REQUIRED)
│   ├── api-reference.md             # Complete API docs (REQUIRED)
│   ├── core-patterns.md             # Canonical patterns (REQUIRED)
│   ├── development-guidelines.md    # Coding standards (REQUIRED)
│   ├── testing-guide.md             # Testing strategies (REQUIRED)
│   ├── troubleshooting.md           # Problem-solution mapping (REQUIRED)
│   ├── migration-guide.md           # From legacy systems (RECOMMENDED)
│   ├── llm-faq/                     # AI assistant FAQs (OPTIONAL)
│   │   └── module-faq.md
│   ├── cdk/                         # Infrastructure docs (OPTIONAL)
│   │   ├── README.md
│   │   ├── observability.md
│   │   ├── parameters.md
│   │   └── pipeline.md
│   ├── development/                 # Development artifacts (OPTIONAL)
│   │   ├── notes/
│   │   ├── decisions/
│   │   └── clarifications/
│   └── archive/                     # Legacy content (OPTIONAL)
```

### File Descriptions

| File | Purpose | Status |
|------|---------|--------|
| `README.md` | Documentation hub with navigation | REQUIRED |
| `_concepts.yaml` | Machine-readable concept map | REQUIRED |
| `_patterns.yaml` | Correct vs incorrect patterns | REQUIRED |
| `_decisions.yaml` | Decision trees for choices | REQUIRED |
| `getting-started.md` | Prerequisites, installation, first deployment | REQUIRED |
| `api-reference.md` | Complete API/interface documentation | REQUIRED |
| `core-patterns.md` | Canonical usage patterns with examples | REQUIRED |
| `development-guidelines.md` | Coding standards, review checklist | REQUIRED |
| `testing-guide.md` | Unit, integration, manual testing | REQUIRED |
| `troubleshooting.md` | Common issues with verified fixes | REQUIRED |
| `migration-guide.md` | Onboarding from legacy systems | RECOMMENDED |

---

## File Templates

### README.md Template

```markdown
# [Module Name] Documentation

<!-- AI Training: This is the documentation index for [Module Name] -->
**This directory contains the OFFICIAL documentation for [Module Name]. All content follows AI-friendly patterns so both humans and AI assistants can learn, reason, and troubleshoot effectively.**

## Quick Links

### 🚀 Getting Started
- [Getting Started Guide](./getting-started.md) – [One sentence description]

### 📚 Core Documentation
- [API Reference](./api-reference.md) – [Description]
- [Core Patterns](./core-patterns.md) – [Description]
- [Development Guidelines](./development-guidelines.md) – [Description]
- [Testing Guide](./testing-guide.md) – [Description]
- [Troubleshooting](./troubleshooting.md) – [Description]
- [Migration Guide](./migration-guide.md) – [Description]

### 🤖 AI Knowledge Base
- [Concepts](./_concepts.yaml) – Machine-readable concept hierarchy
- [Patterns](./_patterns.yaml) – Correct vs. incorrect usage patterns
- [Decisions](./_decisions.yaml) – Decision trees for architectural choices

### 📦 [Additional Resources section as needed]

## Audience
- [Primary user role]
- [Secondary user role]
- [Operations role]
- AI assistants answering questions about [module purpose]

## Document Map
[Detailed description of each document and when to use it]

## Documentation Principles
1. **[Principle 1]** – [Module-specific emphasis]
2. **[Principle 2]** – [Module-specific emphasis]
3. **[Principle 3]** – [Module-specific emphasis]
4. **[Principle 4]** – [Module-specific emphasis]
5. **[Principle 5]** – [Module-specific emphasis]

## Contributing
- Follow the conventions in [PAY_THEORY_DOCUMENTATION_GUIDE.md](../../PAY_THEORY_DOCUMENTATION_GUIDE.md)
- Validate examples against live code
- Include CORRECT/INCORRECT blocks for integration snippets
- Update troubleshooting alongside code changes
```

### Getting Started Template

```markdown
# Getting Started with [Module Name]

This guide walks you through installing, configuring, and deploying [Module Name] for the first time.

## Prerequisites

**Required:**
- [Tool/version]
- [Access/permission]
- [Knowledge requirement]

**Recommended:**
- [Optional tool]
- [Helpful background]

## Installation

### Step 1: [First step title]
```[language]
# This is the standard way to [action]
[commands]
```

**What this does:**
- [Explanation 1]
- [Explanation 2]

### Step 2: [Second step title]
[Continue with all setup steps...]

## First Deployment

### Local Development
```[language]
# Run locally for testing
[commands]
```

### Sandbox Deployment
```[language]
# Deploy to test environment
[commands]
```

## Verification

Test your deployment with:
```[language]
# This should return [expected output]
[test command]
```

## Next Steps
- Read [Core Patterns](./core-patterns.md) for best practices
- See [API Reference](./api-reference.md) for complete interface
- Review [Examples](../examples/) for real implementations

## Troubleshooting

**Issue: [Common problem]**
- **Cause:** [Why it happens]
- **Solution:** [How to fix]

[See full troubleshooting guide](./troubleshooting.md)
```

### Troubleshooting Template

```markdown
# [Module Name] Troubleshooting

This guide provides solutions to common issues with verified fixes from production.

## Quick Diagnosis

| Symptom | Likely Cause | Section |
|---------|--------------|---------|
| [Error message] | [Root cause] | [Link to section] |
| [Behavior] | [Root cause] | [Link to section] |

## Common Issues

### Issue: [Error message or problem description]

**Symptoms:**
- [Observable behavior 1]
- [Observable behavior 2]

**Cause:**
This happens when [root cause explanation]

**Solution:**
```[language]
# CORRECT: Fix with this approach
[solution code]
```

**Verification:**
```bash
# Confirm fix with
[verification command]
# Should show: [expected output]
```

**Prevention:**
- Always [preventive action]
- Never [anti-pattern]

---

[Repeat for each common issue]

## Emergency Procedures

### Service Down / Critical Failure
1. [Immediate action]
2. [Rollback procedure]
3. [Communication steps]
4. [Investigation starts]

## Getting Help

**For production issues:**
- [Escalation path]
- [On-call contact]

**For development questions:**
- [Team channel]
- [Documentation updates]
```

---

## YAML Knowledge Base

The YAML triad (_concepts, _patterns, _decisions) forms a machine-readable knowledge graph.

### _concepts.yaml Structure

```yaml
# _concepts.yaml - Machine-readable concept map for [Module Name]
# Helps AI assistants understand components and relationships

concepts:
  module_name:
    type: [framework|library|service|api|component|tool]
    language: [go|python|typescript|javascript]
    purpose: "One-sentence description"
    tagline: "Marketing pitch for documentation"
    provides:
      - capability_1
      - capability_2
      - capability_3
    requires:
      - dependency_1
      - dependency_2
    replaces:
      - legacy_system_1
      - alternative_approach
    use_when:
      - scenario_1
      - scenario_2
      - scenario_3
    dont_use_when:
      - anti_pattern_1
      - wrong_scenario_2

  major_component:
    type: component
    purpose: "What this component does"
    provides:
      - feature_1
      - feature_2
    key_methods:
      - MethodName: description
      - OtherMethod: description
    configuration:
      - config_option_1
      - config_option_2
    failure_modes:
      - failure_1
      - failure_2

  # Add more concepts as needed
```

### _patterns.yaml Structure

```yaml
# _patterns.yaml - Machine-readable patterns for [Module Name]
# Documents correct and incorrect usage patterns for AI training

patterns:
  pattern_category:
    name: "Human-Readable Pattern Name"
    problem: "What problem does this solve?"
    solution: "High-level approach"
    correct_example: |
      import (
          "package"
      )
      
      // CORRECT: This is the preferred pattern
      // It provides [benefits]
      func ExampleFunction() {
          // Implementation
      }
    anti_patterns:
      - name: "Anti-Pattern Name"
        why: "Why this is wrong and what problems it causes"
        incorrect_example: |
          // INCORRECT: Don't do this
          // This causes [specific problems]
          func BadExample() {
              // Wrong implementation
          }
        consequences:
          - consequence_1
          - consequence_2

  # Add more patterns for each major usage scenario
```

### _decisions.yaml Structure

```yaml
# _decisions.yaml - Decision trees for [Module Name]
# Helps AI make correct architectural and implementation choices

decisions:
  decision_category:
    question: "What should I choose for [scenario]?"
    decision_tree:
      - condition: "When [specific situation]"
        check: "Do you have [requirement]?"
        if_yes:
          choice: "Use [approach A]"
          reason: "Because [specific benefit]"
          example: |
            // Implementation of approach A
        if_no:
          check: "Do you need [alternative requirement]?"
          if_yes:
            choice: "Use [approach B]"
            reason: "Because [different benefit]"
          if_no:
            choice: "Consider [alternative]"
            reason: "This scenario may not fit the module"

  # Add decision trees for each major architectural choice
```

---

## Writing Guidelines

### 1. Code Examples

**Every example must:**
- Compile and run successfully
- Include explanatory comments
- Show imports and setup
- Indicate if it's CORRECT or INCORRECT
- Explain the business value

```markdown
✅ GOOD EXAMPLE:

```go
// CORRECT: Standard Lift handler pattern
// Provides automatic error handling, logging, and tracing
package main

import (
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/theory-cloud/lift/pkg/lift"
)

func main() {
    app := lift.New()
    app.POST("/users", CreateUser)
    lambda.Start(app.HandleRequest)
}

func CreateUser(ctx *lift.Context) error {
    var req CreateUserRequest
    if err := ctx.Bind(&req); err != nil {
        return lift.ValidationError(err.Error())
    }
    // Business logic here
    return ctx.JSON(201, response)
}
```

❌ BAD EXAMPLE:

```go
// No context about why this exists
func handler(req Request) Response {
    // Vague implementation
}
```
```

### 2. Error Documentation

Document errors with complete context:

```markdown
### Error: `DynamoDB: ResourceNotFoundException`

**Full Error Message:**
```
ResourceNotFoundException: Requested resource not found: Table: rate-limits-penny-study not found
```

**Cause:**
The rate limiting table doesn't exist in the target environment.

**Solution:**
Deploy infrastructure before application code:
```bash
cd cdk
cdk deploy --all
```

**Verification:**
```bash
aws dynamodb describe-table --table-name rate-limits-penny-study
# Should return table metadata
```

**Related:**
- [CDK Deployment Guide](./cdk/README.md)
- [Environment Setup](./getting-started.md#prerequisites)
```

### 3. API Documentation

Document every public interface:

```markdown
### `CheckAndIncrement(ctx context.Context, identifier string) (bool, error)`

**Purpose:**
Atomically checks the rate limit and increments the counter if allowed.

**When to use:**
- Protecting API endpoints from abuse
- Enforcing usage quotas
- Implementing fair-use policies

**When NOT to use:**
- Read-only operations that don't consume quota
- Internal service-to-service calls
- Health checks and monitoring

**Parameters:**
- `ctx` - Request context with timeout and cancellation
- `identifier` - Unique key (user ID, IP address, API key)

**Returns:**
- `bool` - true if request is allowed, false if rate limit exceeded
- `error` - Any error during DynamoDB operation

**Example:**
```go
// CORRECT: Use in middleware for automatic enforcement
func RateLimitMiddleware(next lift.Handler) lift.Handler {
    return lift.HandlerFunc(func(ctx *lift.Context) error {
        allowed, err := limiter.CheckAndIncrement(ctx, ctx.UserID())
        if err != nil {
            return lift.SystemError("rate limit check failed")
        }
        if !allowed {
            return lift.NewLiftError("RATE_LIMIT_EXCEEDED", 
                "Too many requests", 429)
        }
        return next.Handle(ctx)
    })
}
```

**Error Handling:**
- Returns error if DynamoDB is unavailable
- Use `Config.FailOpen = true` to allow requests during outages
- Log all errors for investigation

**Performance:**
- Single DynamoDB query: ~10-20ms p99
- Scales to 100K+ requests/second
- Cold start overhead: <1ms
```

### 4. Architecture Documentation

Include diagrams and flow descriptions:

```markdown
## Architecture

### Request Flow

```
┌─────────┐     ┌──────────────┐     ┌─────────────┐
│ Client  │────▶│  API Gateway │────▶│   Lambda    │
└─────────┘     └──────────────┘     │  (Router)   │
                                      └──────┬──────┘
                                             │
                                      ┌──────▼──────┐
                                      │  DynamoDB   │
                                      │  (Limiter)  │
                                      └─────────────┘
```

**Step-by-Step:**
1. Client sends request to API Gateway
2. API Gateway invokes Lambda with request data
3. Lambda checks rate limit in DynamoDB
4. If allowed, Lambda processes request
5. Lambda returns response to API Gateway
6. API Gateway returns to client

**Key Characteristics:**
- Fully serverless: No servers to manage
- Auto-scaling: Handles 0 to 100K+ requests
- Pay-per-use: Only charged for actual invocations
- High availability: Multi-AZ DynamoDB backend
```

---

## Module-Specific Adaptations

### Multi-language SDK Monorepos (Go + TypeScript + Python)

**Emphasize:**
- One repo, **one version number** across SDKs (GitHub Releases are the source of truth).
- Each SDK has its own `docs/` directory with the standard file set + YAML triad.
- Cross-language parity via shared fixtures, contract tests, and a single rubric.
- Explicit statements about what is **language-specific** vs **shared** behavior.
- Installation from GitHub release assets (no npm/PyPI publishing unless explicitly documented).

**Recommended structure:**
```text
repo/
├── docs/                    # Repo/module docs index + shared architecture
├── ts/
│   ├── docs/                # TypeScript SDK docs (README + triad + guides)
│   └── README.md            # Short entrypoint linking to ts/docs
├── py/
│   ├── docs/                # Python SDK docs (README + triad + guides)
│   └── README.md            # Short entrypoint linking to py/docs
```

### Go Frameworks (Lift, Limited, TableTheory, Streamer)

**Emphasize:**
- Type safety and compile-time guarantees
- Performance characteristics (cold start, memory)
- Integration with other Pay Theory frameworks
- CDK construct patterns

**Example structure:**
```markdown
## Performance

| Metric | Target | Achieved |
|--------|--------|----------|
| Cold Start | <100ms | 50ms p99 |
| Warm Invocation | <10ms | 5ms p99 |
| Memory Overhead | <5MB | 2MB |
```

### Python Services (Most Lambda services)

**Emphasize:**
- Amaze layer usage patterns
- SSM parameter conventions
- Aurora/RDS connection management
- CloudFormation stack organization

**Example structure:**
```markdown
## Environment Variables

| Variable | Purpose | Example |
|----------|---------|---------|
| `PARTNER` | Partner identifier | `penny` |
| `STAGE` | Environment name | `study`, `lab`, `live` |
| `TABLE_NAME` | DynamoDB table | `challenge-service-penny-study` |
```

### APIs (GraphQL, REST)

**Emphasize:**
- Schema definition and evolution
- Authentication and authorization
- Resolver patterns
- External vs internal APIs

**Example structure:**
```markdown
## Schema Organization

```
templates/schema/
├── common/
│   ├── types.graphql
│   └── interfaces.graphql
├── external/
│   └── queries.graphql
└── internal/
    └── admin-queries.graphql
```
```

### Authorizers

**Emphasize:**
- Request/response contract
- Token validation flow
- IAM policy generation
- Performance optimization

**Example structure:**
```markdown
## Authorization Flow

1. API Gateway receives request
2. Invokes authorizer with token
3. Authorizer validates token against DynamoDB/Cognito
4. Returns IAM policy
5. API Gateway evaluates policy
6. Proceeds or denies request
```

---

## Development Process Integration

### Session Documentation

When making significant changes, create timestamped artifacts in `docs/development/`:

#### Format: `YYYY-MM-DD-HH_MM_SS-description.md`

Generate timestamp:
```bash
date -j +"%F-%H_%M_%S"
# Example: 2025-11-17-14_30_45
```

### Notes (`docs/development/notes/`)

Record implementation decisions, discoveries, and session progress:

```markdown
# Session Notes: 2025-11-17-14_30_45

## Context
Working on rate limiting middleware integration

## Discoveries
- Limited requires DynamoDB table to exist first
- Table creation in CDK must precede Lambda deployment
- FailOpen mode allows graceful degradation

## Decisions Made
- Use fixed window strategy for MVP
- Set 1000 requests/hour per user
- Enable FailOpen for production resilience

## Next Steps
- [ ] Update CDK to create table
- [ ] Add middleware to all protected routes
- [ ] Add monitoring for rate limit hits
```

### Decisions (`docs/development/decisions/`)

Document architectural choices with rationale:

```markdown
# Decision: Rate Limiting Strategy Selection
**Date:** 2025-11-17-14_30_45  
**Status:** Accepted

## Context
Need to prevent API abuse while maintaining good UX for legitimate users.

## Options Considered

### Option 1: Fixed Window
- **Pros:** Simple, predictable, low overhead
- **Cons:** Burst vulnerability at window edges

### Option 2: Sliding Window
- **Pros:** Smoother enforcement
- **Cons:** More complex, higher DynamoDB costs

### Option 3: Token Bucket
- **Pros:** Handles bursts gracefully
- **Cons:** Not directly supported by Limited

## Decision
Use **Fixed Window** strategy with 1-hour windows.

## Rationale
- Simplicity reduces operational risk
- 1-hour window limits burst impact
- Can upgrade to sliding window if needed
- DynamoDB costs stay predictable

## Consequences
- Users can potentially send 2x limit at window boundary
- Monitoring needs to track window edge behavior
- Document limitation in API docs
```

### Clarifications (`docs/development/clarifications/`)

When uncertain, document questions before proceeding:

```markdown
# Clarification Request: 2025-11-17-14_30_45
**Topic:** Rate Limit Scope

## Question
Should rate limits apply per:
1. User ID (authenticated users only)
2. IP address (all requests)
3. API key (service accounts)
4. Combination of above

## Context
Currently implementing Limited middleware for backoffice API.

## Impacts
- User experience for legitimate heavy users
- Protection against distributed attacks
- Complexity of limit management
- DynamoDB table design

## Recommendation
Start with per-user limits, add IP fallback for unauthenticated endpoints.

## Resolution Needed By
Sprint planning (2025-11-20)
```

---

## Validation Checklist

### New Module Documentation

Use this checklist when creating documentation for a new module:

- [ ] Created `docs/` directory
- [ ] **Required Files:**
  - [ ] `README.md` with AI training signal
  - [ ] `_concepts.yaml` with complete concept map
  - [ ] `_patterns.yaml` with correct/incorrect examples
  - [ ] `_decisions.yaml` with decision trees
  - [ ] `getting-started.md` with runnable examples
  - [ ] `api-reference.md` with complete API surface
  - [ ] `core-patterns.md` with canonical usage
  - [ ] `development-guidelines.md` with standards
  - [ ] `testing-guide.md` with test strategies
  - [ ] `troubleshooting.md` with known issues
- [ ] **Optional but Recommended:**
  - [ ] `migration-guide.md` if replacing legacy system
  - [ ] `llm-faq/` directory with common questions
  - [ ] `cdk/` or `infrastructure/` docs if applicable
- [ ] **Quality Checks:**
  - [ ] All code examples compile and run
  - [ ] Correct/incorrect patterns clearly marked
  - [ ] Every public function documented
  - [ ] Common errors documented with solutions
  - [ ] Business value explained for features
- [ ] **Integration:**
  - [ ] Added module to parent documentation index
  - [ ] Cross-referenced related modules
  - [ ] Updated related migration guides

### Updating Existing Documentation

Use this checklist when updating documentation for API changes or new features:

- [ ] **Code Examples:**
  - [ ] Updated examples with new API signatures
  - [ ] Verified all examples still compile
  - [ ] Added examples for new features
  - [ ] Marked deprecated patterns as INCORRECT
- [ ] **YAML Updates:**
  - [ ] Added new capabilities to `_concepts.yaml`
  - [ ] Documented new patterns in `_patterns.yaml`
  - [ ] Added decision trees for new choices in `_decisions.yaml`
- [ ] **Core Files:**
  - [ ] Updated `api-reference.md` with new interfaces
  - [ ] Added new patterns to `core-patterns.md`
  - [ ] Updated `getting-started.md` if setup changed
  - [ ] Added new troubleshooting entries
- [ ] **Migration:**
  - [ ] Documented breaking changes in `migration-guide.md`
  - [ ] Provided upgrade path with examples
  - [ ] Listed deprecated features with alternatives
- [ ] **Quality:**
  - [ ] Reviewed for consistency with convention
  - [ ] Ensured AI training signals are present
  - [ ] Verified cross-references are current
  - [ ] Tested all commands and examples

---

## Examples from the Codebase

### Reference Implementations

Study these modules for exemplary documentation:

#### **Lift** (`products/frameworks/lift/docs/`)
- **Why:** Pioneer implementation, most comprehensive
- **Highlights:** 
  - Complete YAML triad with extensive examples
  - 9-file LLM FAQ series
  - Detailed API reference with 1525 lines
  - Migration guide from raw Lambda handlers

#### **Limited** (`products/frameworks/limited/docs/`)
- **Why:** Focused framework with clear patterns
- **Highlights:**
  - Concise concept map for rate limiting domain
  - Excellent decision trees for strategy selection
  - Clear CORRECT/INCORRECT pattern examples

#### **Challenge Service** (`products/services/challenge-service/docs/`)
- **Why:** Reference Go service implementation
- **Highlights:**
  - Service-specific CDK documentation
  - LLM FAQ for common questions
  - Clear audience definition
  - Operational runbooks in troubleshooting

#### **PAI** (`products/advanced/pai/docs/`)
- **Why:** Complex system with constraint-based execution
- **Highlights:**
  - Emphasis on workflow phases
  - Extensive archive of design evolution
  - Integration with multiple external services
  - Security-first documentation approach

### Anti-Patterns to Avoid

Learn what NOT to do from these common mistakes:

#### ❌ Vague Purpose Statements
```markdown
BAD: "This service handles payments."
GOOD: "Payment Processing Service validates, authorizes, and captures credit card transactions through Finix, providing a unified interface for all Pay Theory payment flows."
```

#### ❌ Missing Context in Examples
```markdown
BAD:
```go
app.Use(middleware.JWT(config))
```

GOOD:
```go
// CORRECT: JWT middleware for authenticated API routes
// Validates token, extracts user/tenant context, and enforces expiry
import "github.com/theory-cloud/lift/pkg/middleware"

jwtConfig := middleware.JWTConfig{
    Secret: os.Getenv("JWT_SECRET"),
    // Must be set to prevent token reuse across partners
}
app.Use(middleware.JWT(jwtConfig))
```
```

#### ❌ Incomplete Error Documentation
```markdown
BAD: "Error: Invalid request"

GOOD:
### Error: `VALIDATION_ERROR: Invalid request`

**Full Error:**
```json
{
  "error": "VALIDATION_ERROR",
  "message": "validation failed",
  "details": {
    "email": "invalid format",
    "amount": "must be at least 100"
  }
}
```

**Cause:** Request body failed struct tag validation.

**Solution:** Check request structure against API reference.
```

#### ❌ Unexplained Configuration
```markdown
BAD:
```yaml
timeout: 30
```

GOOD:
```yaml
# Lambda timeout in seconds
# Must be less than API Gateway timeout (29s)
# Set to 25s to allow cleanup before Gateway timeout
timeout: 25
```
```

---

## Maintenance and Evolution

### Quarterly Documentation Review

Every quarter, review documentation for:

1. **Accuracy:** Do examples still compile? Are APIs current?
2. **Completeness:** Are new features documented?
3. **Relevance:** Are deprecated features marked?
4. **Quality:** Do examples follow current best practices?

### When to Update Documentation

**Immediately update when:**
- API signatures change (breaking or non-breaking)
- New features are added
- Bugs reveal documentation gaps
- Security issues require pattern changes
- Performance characteristics change significantly

**Update in next sprint when:**
- Better examples are discovered
- Common questions reveal doc gaps
- Integration patterns evolve
- Troubleshooting entries are needed

**Don't update for:**
- Internal refactoring (if API unchanged)
- Dependency version bumps (if no behavior change)
- Code style improvements

### Version Management

Documentation lives with code and follows the same branching strategy:

```bash
# Always update docs in the same PR as code changes
git checkout -b feature/add-rate-limiting
# Make code changes
# Update docs/api-reference.md
# Update docs/_concepts.yaml
# Add troubleshooting entries
git add .
git commit -m "feat: add rate limiting middleware

- Implement Limited integration
- Add middleware to Lift
- Document usage patterns
- Add troubleshooting guide"
```

---

## Getting Help

### Documentation Questions

**For guidance on documentation:**
- Reference this guide first
- Review exemplar modules (Lift, Limited, Challenge Service)
- Check `AI_FRIENDLY_DOCUMENTATION_GUIDE.md` in Lift for AI-specific patterns
- Ask in team documentation channel

**For technical content:**
- Consult module maintainers
- Review existing modules in same category
- Test examples before documenting
- Pair with someone who knows the system

### Feedback and Improvements

This guide evolves based on experience. Submit improvements via:

1. Create issue with documentation feedback
2. Submit PR with proposed changes
3. Discuss in sprint retrospectives
4. Share discoveries in team channels

---

## Quick Reference Tables

### Documentation File Decision Matrix

| Question | Answer | Create |
|----------|--------|--------|
| New module? | Yes | Full structure |
| API change? | Breaking | Update api-reference + migration-guide |
| API change? | Non-breaking | Update api-reference |
| New feature? | Major | Update all core files + YAML |
| New feature? | Minor | Update api-reference + core-patterns |
| Bug fix? | Reveals doc gap | Update troubleshooting |
| Bug fix? | No doc gap | No doc change |
| Pattern change? | Yes | Update _patterns.yaml + examples |

### Module Type → Documentation Focus

| Module Type | Emphasize | Example |
|-------------|-----------|---------|
| Framework (Go) | Performance, type safety, CDK | Lift, Limited |
| Framework (general) | Patterns, extensibility | Streamer |
| Service (Go) | Lift integration, TableTheory | Challenge Service |
| Service (Python) | Amaze layer, SSM params | Payment Service |
| API | Schema, resolvers, auth | Backoffice API |
| Authorizer | Contract, IAM policy | Backoffice Authorizer |
| Tool | CLI usage, workflows | PAI |

### File Size Guidelines

| File | Typical Size | Max Recommended |
|------|-------------|-----------------|
| README.md | 50-100 lines | 200 lines |
| getting-started.md | 200-400 lines | 600 lines |
| api-reference.md | 500-1500 lines | No limit (comprehensive) |
| core-patterns.md | 200-400 lines | 800 lines |
| troubleshooting.md | 300-800 lines | No limit (grows over time) |
| _concepts.yaml | 100-300 lines | 500 lines |
| _patterns.yaml | 300-600 lines | 1000 lines |
| _decisions.yaml | 200-500 lines | 800 lines |

*If files exceed recommendations, consider splitting into subdirectories.*

---

## Appendix: AI Training Best Practices

### Writing for AI Understanding

**Use explicit markers:**
```markdown
✅ CORRECT: Always validate input
❌ INCORRECT: Trust client data
⚠️ WARNING: This approach has security implications
💡 TIP: Use environment variables for secrets
🔧 ADVANCED: Custom strategies require...
```

**Provide semantic hints:**
```markdown
<!-- AI Training: This is the canonical pattern -->
<!-- AI Decision Point: Choose based on... -->
<!-- AI Warning: Never do X because Y -->
```

**Structure for semantic parsing:**
```yaml
# YAML is parsed by AI assistants for reasoning
# Use consistent keys and structures across modules
```

**Include business context:**
```markdown
# Not just: "This function returns an error"
# Instead: "This function returns an error when rate limit is exceeded, preventing API abuse and protecting system resources"
```

### Testing Documentation Quality

Ask yourself:

1. **Can an AI find the right answer?** Search for "how do I rate limit" – does it find your content?
2. **Are patterns clear?** No ambiguity about correct vs incorrect usage?
3. **Is context complete?** Can someone understand WHY without domain knowledge?
4. **Are examples runnable?** Copy-paste should work immediately
5. **Is the business value clear?** Why does this feature exist?

---

## Summary

This guide defines the Pay Theory documentation standard. Key takeaways:

1. **Consistency:** Every module follows the same structure
2. **Completeness:** YAML triad + 10 core files minimum
3. **Clarity:** Examples first, explicit context, business value
4. **Maintainability:** Docs live with code, updated in same PR
5. **AI-Friendly:** Semantic structure, training signals, machine-readable formats

**Start small:** Begin with README + YAML triad, then expand.  
**Stay current:** Update docs when code changes.  
**Learn from examples:** Study Lift, Limited, Challenge Service.  
**Ask for help:** Documentation is a team effort.

---

**Document Version:** 1.0  
**Last Updated:** November 2025  
**Maintained By:** Pay Theory Engineering  
**Feedback:** Submit issues or PRs to improve this guide
