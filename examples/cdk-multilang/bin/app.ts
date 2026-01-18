import { App } from 'aws-cdk-lib';

import { MultilangDemoStack } from '../lib/multilang-demo-stack';

const app = new App();

new MultilangDemoStack(app, 'TheorydbMultilangDemoStack');

