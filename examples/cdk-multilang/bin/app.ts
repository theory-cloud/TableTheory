import { App } from 'aws-cdk-lib';

import { MultilangDemoStack } from '../lib/multilang-demo-stack';

// CDK_SYNTH_NO_BUNDLING is set by the examples harness when it asserts the
// synthesized stack shape. Bundling the Go/Node/Python assets is irrelevant to
// that shape and needs toolchains and network the assertion must not depend on;
// a real deploy leaves it unset and bundles normally.
const context =
  process.env.CDK_SYNTH_NO_BUNDLING === '1'
    ? { 'aws:cdk:bundling-stacks': [] }
    : {};

const app = new App({ context });

new MultilangDemoStack(app, 'TheorydbMultilangDemoStack');

