export type DmsVersion = "0.1";

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
}

export interface Step {
  op:
    | "create"
    | "get"
    | "update"
    | "delete"
    | "query"
    | "scan"
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
  actual?: TransitionActual;
  event?: TransitionEvent;
  ms?: number;
  save?: Record<string, string>;
  expect?: Expectation;
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
  item_contains?: Record<string, unknown>;
  items_contains?: Array<Record<string, unknown>>;
  item_count?: number;
  item_has_fields?: string[];
  item_missing_fields?: string[];
  raw_attribute_types?: Record<string, string>;
  item_field_equals_var?: Record<string, string>;
  item_field_not_equals_var?: Record<string, string>;
}
