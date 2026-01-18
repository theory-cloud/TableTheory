# Contributing to TableTheory

First off, thank you for considering contributing to TableTheory! It's people like you that make TableTheory such a great tool. We welcome contributions from everyone, regardless of their experience level.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [How Can I Contribute?](#how-can-i-contribute)
- [Development Setup](#development-setup)
- [Pull Request Process](#pull-request-process)
- [Style Guide](#style-guide)
- [Testing Guidelines](#testing-guidelines)
- [Documentation](#documentation)
- [Community](#community)

## Code of Conduct

This project and everyone participating in it is governed by the [TableTheory Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to [conduct@theorydb.io](mailto:conduct@theorydb.io).

## Getting Started

Before you begin:
- Read our [documentation](docs/)
- Check out the [open issues](https://github.com/theorydb/theorydb/issues)
- Join our [community discussions](https://github.com/theorydb/theorydb/discussions)

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check existing issues as you might find out that you don't need to create one. When you are creating a bug report, please include as many details as possible:

- **Use a clear and descriptive title**
- **Describe the exact steps to reproduce the problem**
- **Provide specific examples to demonstrate the steps**
- **Describe the behavior you observed and expected**
- **Include logs, stack traces, and code samples**
- **Include your environment details** (Go version, OS, etc.)

### Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. When creating an enhancement suggestion:

- **Use a clear and descriptive title**
- **Provide a detailed description of the suggested enhancement**
- **Provide specific examples to demonstrate the enhancement**
- **Describe the current behavior and expected behavior**
- **Explain why this enhancement would be useful**

### Your First Code Contribution

Unsure where to begin? Look for these labels:

- `good first issue` - Good for newcomers
- `help wanted` - Extra attention is needed
- `documentation` - Documentation improvements

## Development Setup

1. **Fork the Repository**
   ```bash
   git clone https://github.com/YOUR_USERNAME/theorydb.git
   cd theorydb
   ```

2. **Set Up Your Development Environment**
   ```bash
   # Install Go 1.21 or later
   # https://golang.org/doc/install

   # Install dependencies
   go mod download

   # Run tests
   make test

   # Offline unit coverage baseline (skips DynamoDB Local)
   make unit-cover

   # Run linter
   make lint
```

`make unit-cover` runs `go test ./... -short -coverpkg=./... -coverprofile=coverage_unit.out` to establish an offline coverage baseline. Use the regular `make test` or other integration/stress targets when you need DynamoDB Local or AWS parity checks.

3. **Create a Branch**
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/your-bug-fix
   ```

## Pull Request Process

1. **Ensure your code follows our style guide** (see below)
2. **Update the documentation** with details of changes
3. **Add tests** for your changes
4. **Ensure all tests pass** with `make test`
5. **Update the CHANGELOG.md** with your changes
6. **Submit your pull request**

### PR Title Format

Use conventional commit format:
- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation only changes
- `style:` Code style changes (formatting, etc.)
- `refactor:` Code refactoring
- `perf:` Performance improvements
- `test:` Adding or updating tests
- `chore:` Maintenance tasks

Example: `feat: add support for conditional updates`

**Release automation note:** this repo uses `release-please`; PRs that update **dependencies** or other release artifacts should use a release-eligible type (recommended: `fix(deps): ...`) so a new rc/release is generated.

### PR Description Template

```markdown
## Description
Brief description of what this PR does.

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing completed

## Checklist
- [ ] My code follows the style guidelines
- [ ] I have performed a self-review
- [ ] I have commented my code where necessary
- [ ] I have updated the documentation
- [ ] My changes generate no new warnings
- [ ] I have added tests
- [ ] All tests pass locally
```

## Style Guide

### Go Code Style

We follow the standard Go style guidelines:

- Run `gofmt` on your code
- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use meaningful variable and function names
- Keep functions focused and small
- Comment exported functions and types
- Handle errors appropriately

### Code Examples

```go
// Good: Clear function name and documentation
// CreateUser creates a new user in the database with the given attributes.
// It returns the created user with generated ID or an error if creation fails.
func CreateUser(ctx context.Context, name, email string) (*User, error) {
    // Implementation
}

// Bad: Unclear naming and no documentation
func cu(n, e string) (*User, error) {
    // Implementation
}
```

### Commit Messages

- Use the present tense ("Add feature" not "Added feature")
- Use the imperative mood ("Move cursor to..." not "Moves cursor to...")
- Limit the first line to 72 characters
- Reference issues and pull requests liberally after the first line

## Testing Guidelines

### Unit Tests

- Write tests for all new functionality
- Maintain test coverage above 80%
- Use table-driven tests where appropriate
- Mock external dependencies

### Integration Tests

- Test real DynamoDB interactions
- Use Docker for local DynamoDB
- Cover edge cases and error scenarios

### Example Test

```go
func TestCreateUser(t *testing.T) {
    tests := []struct {
        name    string
        input   User
        want    *User
        wantErr bool
    }{
        {
            name: "valid user",
            input: User{Name: "John", Email: "john@example.com"},
            want: &User{ID: "123", Name: "John", Email: "john@example.com"},
            wantErr: false,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := CreateUser(context.Background(), tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("CreateUser() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            // More assertions...
        })
    }
}
```

## Documentation

- Update README.md if needed
- Add/update package documentation
- Include examples in documentation
- Update API documentation
- Add guides for new features

### Documentation Style

- Use clear, simple language
- Include code examples
- Explain the "why" not just the "how"
- Keep it up to date

## Community

- **Discussions**: [GitHub Discussions](https://github.com/theorydb/theorydb/discussions)
- **Discord**: [Join our Discord](https://discord.gg/theorydb)
- **Twitter**: [@theorydb](https://twitter.com/theorydb)

## Recognition

Contributors will be recognized in:
- The CONTRIBUTORS file
- The project README
- Release notes

## Questions?

Feel free to:
- Open an issue with your question
- Start a discussion
- Reach out on Discord

Thank you for contributing to TableTheory! 🎉 
