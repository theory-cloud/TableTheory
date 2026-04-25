import { TheorydbError } from './errors.js';
import type { Model } from './model.js';

export interface WritePolicyOptions {
  protectedAttributes?: readonly string[];
}

export function isWriteOnceModel(model: Model): boolean {
  return model.writePolicy.mode === 'write_once';
}

export function assertMutableWritePolicy(
  model: Model,
  operation: string,
): void {
  if (!isWriteOnceModel(model)) return;
  throw new TheorydbError('ErrImmutableModelMutation', operation);
}

export function assertProtectedFieldsCanMutate(
  model: Model,
  fields: readonly string[],
  extraProtected: readonly string[] = [],
): void {
  const protectedAttributes = protectedAttributeSet(model, extraProtected);
  if (protectedAttributes.size === 0) return;

  for (const field of fields) {
    const attr = rootAttributeName(field);
    if (attr && protectedAttributes.has(attr)) {
      throw new TheorydbError('ErrProtectedFieldMutation', attr);
    }
  }
}

export function assertRawUpdateExpressionAllowed(
  model: Model,
  updateExpression: string,
  expressionAttributeNames?: Record<string, string>,
): void {
  assertMutableWritePolicy(model, 'transaction update');
  assertProtectedFieldsCanMutate(
    model,
    fieldsMutatedByUpdateExpression(updateExpression, expressionAttributeNames),
  );
}

function protectedAttributeSet(
  model: Model,
  extraProtected: readonly string[],
): Set<string> {
  const protectedAttributes = new Set<string>();
  for (const attr of model.writePolicy.protectedAttributes) {
    const root = rootAttributeName(attr);
    if (root) protectedAttributes.add(root);
  }
  for (const attr of extraProtected) {
    const root = rootAttributeName(attr);
    if (root) protectedAttributes.add(root);
  }
  return protectedAttributes;
}

function fieldsMutatedByUpdateExpression(
  updateExpression: string,
  expressionAttributeNames: Record<string, string> = {},
): string[] {
  const fields: string[] = [];
  const sections = updateExpression.matchAll(
    /\b(SET|REMOVE|ADD|DELETE)\b([\s\S]*?)(?=\b(?:SET|REMOVE|ADD|DELETE)\b|$)/gi,
  );

  for (const section of sections) {
    const op = section[1]?.toUpperCase();
    const body = section[2] ?? '';
    for (const clause of splitTopLevelCommas(body)) {
      const root = rootTokenForClause(op, clause);
      const resolved = resolveAttributeName(root, expressionAttributeNames);
      if (resolved) fields.push(resolved);
    }
  }

  return fields;
}

function splitTopLevelCommas(value: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let start = 0;

  for (let i = 0; i < value.length; i++) {
    const ch = value[i];
    if (ch === '(') depth++;
    else if (ch === ')' && depth > 0) depth--;
    else if (ch === ',' && depth === 0) {
      parts.push(value.slice(start, i).trim());
      start = i + 1;
    }
  }

  parts.push(value.slice(start).trim());
  return parts.filter((part) => part.length > 0);
}

function rootTokenForClause(op: string | undefined, clause: string): string {
  if (op === 'SET') {
    return rootAttributeName(clause.split('=', 1)[0] ?? '');
  }
  return rootAttributeName(clause.split(/\s+/, 1)[0] ?? '');
}

function resolveAttributeName(
  token: string,
  expressionAttributeNames: Record<string, string>,
): string {
  if (!token.startsWith('#')) return token;
  const match = token.match(/^#[A-Za-z0-9_]+/);
  if (!match) return token;
  const resolved = expressionAttributeNames[match[0]];
  if (!resolved) return token;
  return resolved + token.slice(match[0].length);
}

function rootAttributeName(field: string): string {
  const trimmed = field.trim();
  if (!trimmed) return '';
  const match = trimmed.match(/^[^\s.[=,)]+/);
  return match?.[0] ?? trimmed;
}
