const MaxFieldNameLength = 255;
const MaxOperatorLength = 20;
const MaxValueStringLength = 400000; // DynamoDB item size limit (approx)
const MaxNestedDepth = 32;
const MaxExpressionLength = 4096;

type SecurityValidationErrorType =
  | 'InjectionAttempt'
  | 'InvalidField'
  | 'InvalidOperator'
  | 'InvalidValue'
  | 'InvalidExpression'
  | 'InvalidTableName'
  | 'InvalidIndexName';

export class SecurityValidationError extends Error {
  readonly type: SecurityValidationErrorType;
  readonly detail: string;

  constructor(type: SecurityValidationErrorType, detail: string) {
    super(`security validation failed: ${type}`);
    this.type = type;
    this.detail = detail;
    this.name = 'SecurityValidationError';
  }
}

export function isSecurityValidationError(
  value: unknown,
): value is SecurityValidationError {
  return value instanceof SecurityValidationError;
}

const dangerousPatterns = [
  "'",
  '"',
  ';',
  '--',
  '/*',
  '*/',
  '<script',
  '</script',
  'eval(',
  'expression(',
  'import(',
  'require(',
];

const sqlKeywords = [
  'union',
  'select',
  'insert',
  'update',
  'delete',
  'drop',
  'alter',
  'exec',
  'execute',
  'script',
  'javascript',
  'vbscript',
];

const legitimateFieldPatterns = [
  /^(created|updated)at$/i,
  /^create(d|r)_?(at|time|date)$/i,
  /^update(d|r)_?(at|time|date)$/i,
  /^delete(d|r)_?(at|time|date|flag)$/i,
  /^insert(ed|er)_?(at|time|date)$/i,
  /^select(ed|or)_?(at|time|date)$/i,
];

const valueScriptPatterns = [
  '<script',
  '</script',
  'eval(',
  'expression(',
  'import(',
  'require(',
  'javascript:',
  'vbscript:',
  'onload=',
  'onerror=',
  'onclick=',
];

const valueSQLInjectionPatterns = [
  "'; drop table",
  "'; delete from",
  "'; update ",
  "'; insert into",
  '"; drop table',
  '"; delete from',
  '"; update ',
  '"; insert into',
  "' or 1=1",
  '" or 1=1',
  "' or '1'='1",
  '" or "1"="1',
  '/**/union/**/select',
  'concat(0x',
  'char(',
  'load_file(',
  '--',
];

const allowedOperators = new Set([
  '=',
  '!=',
  '<>',
  '<',
  '<=',
  '>',
  '>=',
  'BETWEEN',
  'IN',
  'BEGINS_WITH',
  'CONTAINS',
  'EXISTS',
  'NOT_EXISTS',
  'ATTRIBUTE_EXISTS',
  'ATTRIBUTE_NOT_EXISTS',
  'EQ',
  'NE',
  'LT',
  'LE',
  'GT',
  'GE',
]);

export {
  MaxExpressionLength,
  MaxFieldNameLength,
  MaxNestedDepth,
  MaxOperatorLength,
  MaxValueStringLength,
};

export function validateFieldName(field: string): void {
  validateFieldNameBasics(field);

  const lower = field.toLowerCase();
  if (containsAnySubstring(lower, dangerousPatterns)) {
    throw new SecurityValidationError(
      'InjectionAttempt',
      'field name contains dangerous pattern',
    );
  }

  validateFieldNameKeywords(lower, field);

  if (containsControlCharacters(field)) {
    throw new SecurityValidationError(
      'InvalidField',
      'field name contains control characters',
    );
  }

  if (field.includes('.')) {
    validateNestedFieldPath(field);
    return;
  }

  validateFieldPart(field);
}

function validateFieldNameBasics(field: string): void {
  if (!field) {
    throw new SecurityValidationError(
      'InvalidField',
      'field name cannot be empty',
    );
  }
  if (field.length > MaxFieldNameLength) {
    throw new SecurityValidationError(
      'InvalidField',
      'field name exceeds maximum length',
    );
  }
}

function validateFieldNameKeywords(fieldLower: string, field: string): void {
  for (const keyword of sqlKeywords) {
    if (!fieldLower.includes(keyword)) continue;
    if (isLegitimateFieldName(field)) continue;
    if (isStandaloneOrSuspiciousKeyword(fieldLower, keyword)) {
      throw new SecurityValidationError(
        'InjectionAttempt',
        'field name contains suspicious content',
      );
    }
  }
}

function isLegitimateFieldName(field: string): boolean {
  return legitimateFieldPatterns.some((p) => p.test(field));
}

function containsControlCharacters(field: string): boolean {
  for (let i = 0; i < field.length; i++) {
    const code = field.charCodeAt(i);
    if ((code >= 0 && code <= 0x1f) || code === 0x7f) return true;
  }
  return false;
}

function validateNestedFieldPath(field: string): void {
  const parts = field.split('.');
  if (parts.length > MaxNestedDepth) {
    throw new SecurityValidationError(
      'InvalidField',
      'nested field depth exceeds maximum',
    );
  }

  for (const part of parts) {
    try {
      validateFieldPart(part);
    } catch {
      throw new SecurityValidationError('InvalidField', 'invalid field part');
    }
  }
}

function isStandaloneOrSuspiciousKeyword(
  fieldLower: string,
  keyword: string,
): boolean {
  if (fieldLower === keyword) return true;

  const suspiciousPatterns = [
    `${keyword};`,
    `;${keyword}`,
    `${keyword} `,
    ` ${keyword}`,
    `${keyword}.`,
    `.${keyword}`,
    `${keyword}-`,
    `-${keyword}`,
  ];

  return suspiciousPatterns.some((p) => fieldLower.includes(p));
}

function validateFieldPart(part: string): void {
  if (!part) {
    throw new SecurityValidationError(
      'InvalidField',
      'field part cannot be empty',
    );
  }

  if (part.includes('[') && part.includes(']')) {
    const open = part.indexOf('[');
    const close = part.lastIndexOf(']');
    if (close <= open) {
      throw new SecurityValidationError(
        'InvalidField',
        'invalid bracket syntax in field part',
      );
    }

    const fieldName = part.slice(0, open);
    const indexPart = part.slice(open + 1, close);
    const remaining = part.slice(close + 1);

    const fieldPattern = /^[a-zA-Z_][a-zA-Z0-9_]*$/;
    if (!fieldPattern.test(fieldName)) {
      throw new SecurityValidationError(
        'InvalidField',
        'field name part must start with letter or underscore and contain only alphanumeric characters and underscores',
      );
    }

    const indexPattern = /^[0-9]+$/;
    if (!indexPattern.test(indexPart)) {
      throw new SecurityValidationError(
        'InvalidField',
        'list index must be a number',
      );
    }

    if (remaining !== '') {
      throw new SecurityValidationError(
        'InvalidField',
        'unexpected characters after list index',
      );
    }

    return;
  }

  const validPattern = /^[a-zA-Z_][a-zA-Z0-9_]*$/;
  if (!validPattern.test(part)) {
    throw new SecurityValidationError(
      'InvalidField',
      'field part must start with letter or underscore and contain only alphanumeric characters and underscores',
    );
  }
}

export function validateOperator(op: string): void {
  if (!op) {
    throw new SecurityValidationError(
      'InvalidOperator',
      'operator cannot be empty',
    );
  }

  if (op.length > MaxOperatorLength) {
    throw new SecurityValidationError(
      'InvalidOperator',
      'operator exceeds maximum length',
    );
  }

  const opUpper = op.trim().toUpperCase();
  if (!allowedOperators.has(opUpper)) {
    throw new SecurityValidationError(
      'InvalidOperator',
      'operator not allowed',
    );
  }

  const opLower = op.toLowerCase();
  if (containsAnySubstring(opLower, dangerousPatterns)) {
    throw new SecurityValidationError(
      'InjectionAttempt',
      'operator contains dangerous pattern',
    );
  }
}

export function validateValue(value: unknown): void {
  if (value === null || value === undefined) return;

  if (typeof value === 'string') {
    validateStringValue(value);
    return;
  }

  if (Array.isArray(value)) {
    validateArrayValue(value);
    return;
  }

  if (typeof value === 'object') {
    validateObjectValue(value as Record<string, unknown>);
    return;
  }

  if (typeof value === 'function' || typeof value === 'symbol') {
    throw new SecurityValidationError('InvalidValue', 'unsupported value type');
  }
}

function validateStringValue(value: string): void {
  if (value.length > MaxValueStringLength) {
    throw new SecurityValidationError(
      'InvalidValue',
      'string value exceeds maximum length',
    );
  }

  const lower = value.toLowerCase();

  if (
    containsAnySubstring(lower, valueScriptPatterns) ||
    (value.includes('/*') && value.includes('*/')) ||
    containsAnySubstring(lower, valueSQLInjectionPatterns) ||
    looksLikeUnionSelectInjection(lower, value)
  ) {
    throw new SecurityValidationError(
      'InjectionAttempt',
      'string value contains dangerous pattern',
    );
  }
}

function validateArrayValue(value: unknown[]): void {
  if (value.length > 100) {
    throw new SecurityValidationError(
      'InvalidValue',
      'slice value exceeds maximum length of 100 items',
    );
  }
  for (const item of value) {
    try {
      validateValue(item);
    } catch {
      throw new SecurityValidationError(
        'InvalidValue',
        'invalid item in collection',
      );
    }
  }
}

function validateObjectValue(value: Record<string, unknown>): void {
  const entries = Object.entries(value);
  if (entries.length > 100) {
    throw new SecurityValidationError(
      'InvalidValue',
      'map value exceeds maximum keys',
    );
  }

  for (const [k, v] of entries) {
    try {
      validateFieldName(k);
    } catch {
      throw new SecurityValidationError('InvalidValue', 'invalid map key');
    }
    try {
      validateValue(v);
    } catch {
      throw new SecurityValidationError('InvalidValue', 'invalid map value');
    }
  }
}

export function validateExpression(expression: string): void {
  if (expression.length > MaxExpressionLength) {
    throw new SecurityValidationError(
      'InvalidExpression',
      'expression exceeds maximum length',
    );
  }

  const lower = expression.toLowerCase();
  if (containsAnySubstring(lower, dangerousPatterns)) {
    throw new SecurityValidationError(
      'InjectionAttempt',
      'expression contains dangerous pattern',
    );
  }

  const sqlInjectionPatterns = [
    'union select',
    'insert into',
    'update set',
    'delete from',
    'drop table',
    'alter table',
    'exec ',
    'execute ',
  ];
  if (containsAnySubstring(lower, sqlInjectionPatterns)) {
    throw new SecurityValidationError(
      'InjectionAttempt',
      'expression contains dangerous pattern',
    );
  }
}

export function validateTableName(name: string): void {
  if (name.length < 3 || name.length > 255) {
    throw new SecurityValidationError(
      'InvalidTableName',
      'table name length invalid',
    );
  }

  const pattern = /^[a-zA-Z0-9_.-]+$/;
  if (!pattern.test(name)) {
    throw new SecurityValidationError(
      'InvalidTableName',
      'table name contains invalid characters',
    );
  }

  const lower = name.toLowerCase();
  if (containsAnySubstring(lower, dangerousPatterns)) {
    throw new SecurityValidationError(
      'InjectionAttempt',
      'table name contains dangerous pattern',
    );
  }
}

export function validateIndexName(name: string): void {
  if (!name) return;

  if (name.length < 3 || name.length > 255) {
    throw new SecurityValidationError(
      'InvalidIndexName',
      'index name length invalid',
    );
  }

  const pattern = /^[a-zA-Z0-9_.-]+$/;
  if (!pattern.test(name)) {
    throw new SecurityValidationError(
      'InvalidIndexName',
      'index name contains invalid characters',
    );
  }
}

function containsAnySubstring(haystack: string, needles: string[]): boolean {
  return needles.some((n) => haystack.includes(n));
}

function looksLikeUnionSelectInjection(lower: string, raw: string): boolean {
  if (!lower.includes('union') || !lower.includes('select')) return false;

  if (
    !lower.includes('union select') &&
    !lower.includes('union all select') &&
    !lower.includes('union/**/select')
  ) {
    return false;
  }

  return (
    lower.includes('from') ||
    lower.includes('*') ||
    raw.endsWith('--') ||
    raw.endsWith(';')
  );
}
