export type DmsVersion = "0.2";

export type ScalarType =
  "S" | "N" | "B" | "BOOL" | "NULL" | "M" | "L" | "SS" | "NS" | "BS";

export interface DmsDocument {
  dms_version: DmsVersion;
  namespace?: string;
  models: DmsModel[];
}

export interface DmsModel {
  name: string;
  table: { name: string };
  naming?: { convention?: "camelCase" | "snake_case" | "dynamorm" };
  keys: {
    partition: { attribute: string; type: "S" | "N" | "B" };
    sort?: { attribute: string; type: "S" | "N" | "B" };
  };
  write_policy?: {
    mode?: "mutable" | "write_once";
    protected_attributes?: string[];
  };
  attributes: Array<{
    attribute: string;
    type: ScalarType;
    required?: boolean;
    optional?: boolean;
    omit_empty?: boolean;
    json?: boolean;
    binary?: boolean;
    format?: string;
    roles?: string[];
    encryption?: unknown;
  }>;
  indexes?: Array<{
    name: string;
    type: "GSI" | "LSI";
    partition: { attribute: string; type: "S" | "N" | "B" };
    sort?: { attribute: string; type: "S" | "N" | "B" };
    projection?: { type: "ALL" | "KEYS_ONLY" | "INCLUDE"; fields?: string[] };
  }>;
}

export interface Scenario {
  name: string;
  dms_version: DmsVersion;
  requires_capabilities?: string[];
  model: string;
  table?: { name?: string };
  steps: Step[];
  encryption?: EncryptionScenarioConfig;
  seed_runtime?: "go" | "ts" | "py" | string;
  seed_steps?: Step[];
  read_steps?: Step[];
}

export interface EncryptionScenarioConfig {
  provider?: "deterministic" | string;
  seed?: string;
}

export interface Step {
  op:
    | "create"
    | "get"
    | "get_optional"
    | "update"
    | "delete"
    | "query"
    | "scan"
    | "count"
    | "transact_get"
    | "batch_get"
    | "batch_write"
    | "transact_write"
    | "sleep"
    | "save"
    | "transition_append_event"
    | "validate_provenance";
  model?: string;
  if_not_exists?: boolean;
  fields?: string[];
  protected_attributes?: string[];
  item?: Record<string, unknown>;
  key?: Record<string, unknown>;
  query?: ReadRequest;
  scan?: ReadRequest;
  count?: CountRequest;
  transact_get?: TransactGetRequest;
  batch_get?: BatchGetRequest;
  batch_write?: BatchWriteRequest;
  transact_write?: TransactWriteRequest;
  actual?: TransitionActual;
  event?: TransitionEvent;
  ms?: number;
  save?: Record<string, string>;
  expect?: Expectation;
}

export interface TransactGetRequest {
  items: KeyedItem[];
}

export interface BatchGetRequest {
  keys: Array<Record<string, unknown>>;
}

export interface BatchWriteRequest {
  puts?: Array<Record<string, unknown>>;
  deletes?: Array<Record<string, unknown>>;
}

export interface TransactWriteRequest {
  actions: TransactWriteAction[];
}

export interface KeyedItem {
  model?: string;
  key: Record<string, unknown>;
}

export interface TransactWriteAction {
  kind: string;
  model?: string;
  item?: Record<string, unknown>;
  key?: Record<string, unknown>;
  set?: Record<string, unknown>;
  condition_expression?: string;
  expression_attribute_names?: Record<string, string>;
  expression_attribute_values?: Record<string, unknown>;
  if_not_exists?: boolean;
}

export interface CountRequest {
  query?: ReadRequest;
  scan?: ReadRequest;
}

export interface ReadRequest {
  index?: string;
  partition?: ReadCondition;
  sort?: ReadCondition;
  filter?: ReadCondition[];
  sort_direction?: "ASC" | "DESC" | "asc" | "desc";
  limit?: number;
  projection?: string[];
  cursor?: string;
  consistent_read?: boolean;
}

export interface ReadCondition {
  attribute: string;
  operator: string;
  value?: unknown;
  values?: unknown[];
}

export interface TransitionActual {
  model: string;
  key: Record<string, unknown>;
  set: Record<string, unknown>;
  expected_version?: number;
}

export interface TransitionEvent {
  model: string;
  item: Record<string, unknown>;
}

export interface Expectation {
  ok?: boolean;
  error?: string;
  errors?: string[];
  item_contains?: Record<string, unknown>;
  item_equals?: Record<string, unknown>;
  raw_item_contains?: Record<string, unknown>;
  items_contains?: Array<Record<string, unknown>>;
  items_missing_fields?: string[];
  item_count?: number;
  count?: number;
  item_absent?: boolean;
  cursor_equals?: string;
  item_has_fields?: string[];
  item_missing_fields?: string[];
  raw_attribute_types?: Record<string, string>;
  item_field_equals_var?: Record<string, string>;
  item_field_not_equals_var?: Record<string, string>;
}
