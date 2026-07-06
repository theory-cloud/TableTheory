---
title: CDK Multi-language Example
---

# CDK multi-language example

The CDK multi-language example demonstrates a DMS-authored table contract that is consumed from Go, TypeScript, and
Python without changing runtime semantics.

- Source: [`examples/cdk-multilang`](https://github.com/theory-cloud/TableTheory/tree/main/examples/cdk-multilang)
- Local proof: [`examples/cdk-multilang/local/run-local.sh`](https://github.com/theory-cloud/TableTheory/blob/main/examples/cdk-multilang/local/run-local.sh)
- Generated construct docs: [CDK generated constructs](../cdk/generated-constructs.md)

The local proof starts an isolated-port DynamoDB Local instance, has Go write an item, then has TypeScript and Python read
that same item through their peer implementations. Encryption is intentionally excluded from the local proof because
TableTheory encrypted fields fail closed without explicit KMS configuration.
