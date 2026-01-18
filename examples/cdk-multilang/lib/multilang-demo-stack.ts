import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

import { CfnOutput, Duration, RemovalPolicy, Stack, type StackProps } from 'aws-cdk-lib';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import * as kms from 'aws-cdk-lib/aws-kms';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import { NodejsFunction } from 'aws-cdk-lib/aws-lambda-nodejs';
import { Construct } from 'constructs';

function repoRootFrom(stackFileDir: string): string {
  return path.resolve(stackFileDir, '../../..');
}

function isMissingCommandError(error: unknown): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error as { code?: unknown }).code === 'ENOENT'
  );
}

export class MultilangDemoStack extends Stack {
  constructor(scope: Construct, id: string, props: StackProps = {}) {
    super(scope, id, props);

    const repoRoot = repoRootFrom(__dirname);
    const demoDmsPath = path.join(repoRoot, 'examples/cdk-multilang/dms/demo.yml');
    if (!fs.existsSync(demoDmsPath)) {
      throw new Error(`Missing DMS fixture: ${demoDmsPath}`);
    }
    const demoDmsB64 = Buffer.from(fs.readFileSync(demoDmsPath, 'utf8'), 'utf8').toString('base64');

    const table = new dynamodb.Table(this, 'DemoTable', {
      partitionKey: { name: 'PK', type: dynamodb.AttributeType.STRING },
      sortKey: { name: 'SK', type: dynamodb.AttributeType.STRING },
      billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
      removalPolicy: RemovalPolicy.DESTROY,
    });

    const key = new kms.Key(this, 'DemoKey', {
      enableKeyRotation: true,
      removalPolicy: RemovalPolicy.DESTROY,
    });

    const goHandlerDir = path.join(repoRoot, 'examples/cdk-multilang/lambdas/go');
    const goFn = new lambda.Function(this, 'GoDemoFn', {
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.X86_64,
      handler: 'bootstrap',
      memorySize: 256,
      timeout: Duration.seconds(10),
      code: lambda.Code.fromAsset(goHandlerDir, {
        bundling: {
          image: lambda.Runtime.PROVIDED_AL2023.bundlingImage,
          command: [
            'bash',
            '-c',
            [
              'set -euo pipefail',
              `cd /asset-input`,
              'GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /asset-output/bootstrap .',
            ].join('\n'),
          ],
          local: {
            tryBundle(outputDir: string): boolean {
              try {
                execFileSync(
                  'go',
                  [
                    'build',
                    '-o',
                    path.join(outputDir, 'bootstrap'),
                    './examples/cdk-multilang/lambdas/go',
                  ],
                  {
                    cwd: repoRoot,
                    env: {
                      ...process.env,
                      GOOS: 'linux',
                      GOARCH: 'amd64',
                      CGO_ENABLED: '0',
                    },
                    stdio: 'inherit',
                  },
                );
                return true;
              } catch (error) {
                if (isMissingCommandError(error)) {
                  return false;
                }
                throw error;
              }
            },
          },
        },
      }),
      environment: {
        TABLE_NAME: table.tableName,
        KMS_KEY_ARN: key.keyArn,
        DMS_MODEL_B64: demoDmsB64,
      },
    });

    const nodeEntry = path.join(repoRoot, 'examples/cdk-multilang/lambdas/node/handler.ts');
    const nodeFn = new NodejsFunction(this, 'NodeDemoFn', {
      runtime: lambda.Runtime.NODEJS_24_X,
      architecture: lambda.Architecture.X86_64,
      entry: nodeEntry,
      handler: 'handler',
      memorySize: 256,
      timeout: Duration.seconds(10),
      bundling: {
        target: 'node24',
      },
      environment: {
        TABLE_NAME: table.tableName,
        KMS_KEY_ARN: key.keyArn,
        DMS_MODEL_B64: demoDmsB64,
      },
    });

    const pyHandlerDir = path.join(repoRoot, 'examples/cdk-multilang/lambdas/python');
    const theorydbPySrc = path.join(repoRoot, 'py/src/theorydb_py');
    const pyFn = new lambda.Function(this, 'PythonDemoFn', {
      runtime: lambda.Runtime.PYTHON_3_14,
      architecture: lambda.Architecture.X86_64,
      handler: 'handler.handler',
      memorySize: 256,
      timeout: Duration.seconds(10),
      code: lambda.Code.fromAsset(pyHandlerDir, {
        bundling: {
          image: lambda.Runtime.PYTHON_3_14.bundlingImage,
          command: [
            'bash',
            '-c',
            [
              'set -euo pipefail',
              'cp -R /asset-input/* /asset-output/',
              'cp -R /theorydb_py /asset-output/theorydb_py',
              'python -m pip install -r /asset-input/requirements.txt -t /asset-output',
            ].join('\n'),
          ],
          local: {
            tryBundle(outputDir: string): boolean {
              fs.cpSync(pyHandlerDir, outputDir, { recursive: true });
              fs.cpSync(theorydbPySrc, path.join(outputDir, 'theorydb_py'), {
                recursive: true,
              });
              try {
                execFileSync(
                  'python3.14',
                  [
                    '-m',
                    'pip',
                    'install',
                    '-r',
                    path.join(pyHandlerDir, 'requirements.txt'),
                    '-t',
                    outputDir,
                  ],
                  { stdio: 'inherit' },
                );
                return true;
              } catch (error) {
                if (isMissingCommandError(error)) {
                  return false;
                }
                throw error;
              }
            },
          },
          volumes: [{ hostPath: theorydbPySrc, containerPath: '/theorydb_py' }],
        },
      }),
      environment: {
        TABLE_NAME: table.tableName,
        KMS_KEY_ARN: key.keyArn,
        DMS_MODEL_B64: demoDmsB64,
      },
    });

    table.grantReadWriteData(goFn);
    table.grantReadWriteData(nodeFn);
    table.grantReadWriteData(pyFn);
    key.grantEncryptDecrypt(goFn);
    key.grantEncryptDecrypt(nodeFn);
    key.grantEncryptDecrypt(pyFn);

    const goUrl = goFn.addFunctionUrl({ authType: lambda.FunctionUrlAuthType.NONE });
    const nodeUrl = nodeFn.addFunctionUrl({ authType: lambda.FunctionUrlAuthType.NONE });
    const pyUrl = pyFn.addFunctionUrl({ authType: lambda.FunctionUrlAuthType.NONE });

    new CfnOutput(this, 'TableName', { value: table.tableName });
    new CfnOutput(this, 'KmsKeyArn', { value: key.keyArn });
    new CfnOutput(this, 'GoFunctionUrl', { value: goUrl.url });
    new CfnOutput(this, 'NodeFunctionUrl', { value: nodeUrl.url });
    new CfnOutput(this, 'PythonFunctionUrl', { value: pyUrl.url });
  }
}
