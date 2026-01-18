export type DmsVersion = "0.1";

export type ScalarType = "S" | "N" | "B" | "BOOL" | "NULL" | "M" | "L" | "SS" | "NS" | "BS";

export interface DmsDocument {
  dms_version: DmsVersion;
  namespace?: string;
  models: DmsModel[];
}

export interface DmsModel {
  name: string;
  table: { name: string };
  naming?: { convention?: "camelCase" | "snake_case" };
  keys: {
    partition: { attribute: string; type: "S" | "N" | "B" };
    sort?: { attribute: string; type: "S" | "N" | "B" };
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
  model: string;
  table?: { name?: string };
  steps: Step[];
}

export interface Step {
  op: "create" | "get" | "update" | "delete" | "sleep";
  if_not_exists?: boolean;
  fields?: string[];
  item?: Record<string, unknown>;
  key?: Record<string, unknown>;
  ms?: number;
  save?: Record<string, string>;
  expect?: Expectation;
}

export interface Expectation {
  ok?: boolean;
  error?: string;
  item_contains?: Record<string, unknown>;
  item_has_fields?: string[];
  item_missing_fields?: string[];
  raw_attribute_types?: Record<string, string>;
  item_field_equals_var?: Record<string, string>;
  item_field_not_equals_var?: Record<string, string>;
}
