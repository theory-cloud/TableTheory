declare const operatorEscapeBrand: unique symbol;

export type KnownOperator =
  | '='
  | '!='
  | '<>'
  | '<'
  | '<='
  | '>'
  | '>='
  | 'BETWEEN'
  | 'IN'
  | 'BEGINS_WITH'
  | 'CONTAINS'
  | 'EXISTS'
  | 'NOT_EXISTS'
  | 'ATTRIBUTE_EXISTS'
  | 'ATTRIBUTE_NOT_EXISTS'
  | 'EQ'
  | 'NE'
  | 'LT'
  | 'LE'
  | 'GT'
  | 'GE'
  | 'between'
  | 'in'
  | 'begins_with'
  | 'contains'
  | 'exists'
  | 'not_exists'
  | 'attribute_exists'
  | 'attribute_not_exists';

export type OperatorEscape = string & {
  readonly [operatorEscapeBrand]: true;
};

export type QueryOperator = KnownOperator | OperatorEscape;

export function unsafeOperator(operator: string): OperatorEscape {
  return operator as OperatorEscape;
}

export interface Page<T = Record<string, unknown>> {
  items: T[];
  cursor?: string;
}

export interface QueryRetryOptions<T = Record<string, unknown>> {
  maxAttempts?: number;
  baseDelayMs?: number;
  maxDelayMs?: number;
  backoffFactor?: number;
  retryOnEmpty?: boolean;
  retryOnError?: boolean;
  verify?: (page: Page<T>) => boolean;
}
