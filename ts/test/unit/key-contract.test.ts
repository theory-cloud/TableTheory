import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { TheorydbError } from '../../src/errors.js';
import {
  evaluateDerivedKey,
  evaluateDerivedKeyDefinition,
  parseDerivedKeyContract,
  verifyDerivedKeyFixtures,
  type DerivedKeyDefinition,
} from '../../src/key-contract.js';

const fixturePath = fileURLToPath(
  new URL(
    '../../../contract-tests/key-contracts/v0.1/theorymcp-derived-keys.yml',
    import.meta.url,
  ),
);
const fixture = parseDerivedKeyContract(readFileSync(fixturePath, 'utf8'));

{
  verifyDerivedKeyFixtures(fixture);

  const counts = new Map(
    fixture.derived_keys.map((key) => [key.name, key.fixtures?.length ?? 0]),
  );
  assert.equal(counts.get('WildcardScope'), 2);
  assert.equal(counts.get('CanonicalPolicyKey'), 3);
  assert.equal(counts.get('CanonicalBindingKey'), 2);
  assert.equal(counts.get('InterfaceScopeKey'), 1);
  assert.equal(counts.get('SkillScopeKey'), 1);
  assert.equal(counts.get('AgentScopeKey'), 2);
  assert.equal(counts.get('EmailBindingSortKey'), 2);
  assert.equal(counts.get('GitHubRepositoryLookupKey'), 1);
  assert.equal(counts.get('ImportSessionScopeKey'), 1);
}

{
  assert.equal(
    evaluateDerivedKey(fixture, 'CanonicalPolicyKey', {
      client_namespace: 'theorycloud',
      agent_id: 'apptheory',
      partner_id: 'keybank',
      kb_name: 'payments',
      effect: 'allow',
      access_mode: 'partner',
      policy_version: 5,
      management_mode: 'auto_manifest_namespace_binding',
    }),
    'ns=theorycloud|agent=apptheory|partner=keybank|kb=payments|effect=allow|mode=partner|v=5|mgmt=auto_manifest_namespace_binding',
  );
}

{
  const manual = 'manual';
  const key: DerivedKeyDefinition = {
    name: 'ExampleKey',
    join: '|',
    inputs: [
      { name: 'scope', optional: true },
      { name: 'mode', optional: true },
    ],
    segments: [
      {
        name: 'scope',
        prefix: 'scope=',
        value: { input: 'scope' },
        transforms: ['trim', 'wildcard_empty'],
      },
      {
        name: 'mode',
        prefix: 'mode=',
        value: { input: 'mode' },
        transforms: ['trim'],
        default: manual,
        optional: true,
        omit_when: { default: true },
      },
    ],
  };

  assert.equal(
    evaluateDerivedKeyDefinition(key, { scope: ' keybank ' }),
    'scope=keybank',
  );
  assert.equal(
    evaluateDerivedKeyDefinition(key, { scope: '', mode: 'auto' }),
    'scope=*|mode=auto',
  );
}

{
  assert.throws(
    () => evaluateDerivedKey(fixture, 'Missing', {}),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrInvalidModel');
      return true;
    },
  );
}

{
  assert.throws(
    () =>
      parseDerivedKeyContract(`
tabletheory_model_contract_version: "0.1"
derived_keys:
  - name: Bad
    join: ""
    segments:
      - value: { input: id }
        transforms: [lower]
`),
    (err) => {
      assert.ok(err instanceof TheorydbError);
      assert.equal(err.code, 'ErrInvalidModel');
      assert.match(err.message, /unsupported transform/);
      return true;
    },
  );
}
