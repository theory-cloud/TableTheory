import fs from "node:fs/promises";
import path from "node:path";
import YAML from "yaml";
import type { DmsDocument, DmsModel, Scenario } from "./types.js";

export async function loadModelsDir(
  modelsDir: string,
): Promise<Map<string, DmsModel>> {
  const entries = await fs.readdir(modelsDir, { withFileTypes: true });
  const models = new Map<string, DmsModel>();

  for (const entry of entries) {
    if (!entry.isFile()) continue;
    if (!entry.name.endsWith(".yml") && !entry.name.endsWith(".yaml")) continue;

    const filePath = path.join(modelsDir, entry.name);
    const raw = await fs.readFile(filePath, "utf8");
    const doc = YAML.parse(raw) as DmsDocument;

    for (const model of doc.models ?? []) {
      if (model?.name) models.set(model.name, model);
    }
  }

  if (models.size === 0) {
    throw new Error(`No models found in ${modelsDir}`);
  }

  return models;
}

export async function loadScenarioFile(filePath: string): Promise<Scenario> {
  const raw = await fs.readFile(filePath, "utf8");
  const scenario = YAML.parse(raw) as Scenario;
  if (!scenario?.name) throw new Error(`Scenario missing name: ${filePath}`);
  if (!scenario?.model) throw new Error(`Scenario missing model: ${filePath}`);
  validateScenario(scenario, filePath);
  return scenario;
}

export async function loadScenariosDir(dir: string): Promise<Scenario[]> {
  const entries = await fs.readdir(dir, { withFileTypes: true });
  const files = entries
    .filter(
      (e) =>
        e.isFile() && (e.name.endsWith(".yml") || e.name.endsWith(".yaml")),
    )
    .map((e) => path.join(dir, e.name))
    .sort();

  const scenarios: Scenario[] = [];
  for (const filePath of files) {
    scenarios.push(await loadScenarioFile(filePath));
  }
  return scenarios;
}

function validateScenario(scenario: Scenario, filePath: string): void {
  if (scenario.seed_runtime) {
    if (
      !Array.isArray(scenario.seed_steps) ||
      scenario.seed_steps.length === 0
    ) {
      throw new Error(`Scenario missing seed_steps: ${filePath}`);
    }
    if (
      !Array.isArray(scenario.read_steps) ||
      scenario.read_steps.length === 0
    ) {
      throw new Error(`Scenario missing read_steps: ${filePath}`);
    }
  } else if (!Array.isArray(scenario.steps) || scenario.steps.length === 0) {
    throw new Error(`Scenario missing steps: ${filePath}`);
  }

  for (const [label, steps] of [
    ["steps", scenario.steps ?? []],
    ["seed_steps", scenario.seed_steps ?? []],
    ["read_steps", scenario.read_steps ?? []],
  ] as const) {
    for (const [index, step] of steps.entries()) {
      validateStep(filePath, label, index, step);
    }
  }
}

function validateStep(
  filePath: string,
  label: string,
  index: number,
  step: Scenario["steps"][number],
): void {
  const prefix = `${filePath} ${label}[${index}] ${step.op}`;
  switch (step.op) {
    case "sleep":
      break;
    case "create":
    case "update":
    case "save":
      requirePlainObject(step.item, `${prefix}: item is required`);
      break;
    case "get":
    case "delete":
      requirePlainObject(step.key, `${prefix}: key is required`);
      break;
    case "query":
      requireReadRequest(step.query, `${prefix}: query`);
      requireReadCondition(step.query?.partition, `${prefix}: query.partition`);
      break;
    case "scan":
      requireReadRequest(step.scan, `${prefix}: scan`);
      break;
    case "transition_append_event":
      requireTransitionActual(step.actual, `${prefix}: actual`);
      requireTransitionEvent(step.event, `${prefix}: event`);
      break;
    case "validate_provenance":
      requirePlainObject(step.item, `${prefix}: item is required`);
      break;
    default:
      throw new Error(
        `${filePath} ${label}[${index}]: unsupported op ${String(step.op)}`,
      );
  }
}

function requireReadRequest(value: unknown, prefix: string): void {
  requirePlainObject(value, `${prefix} is required`);
  const request = value as {
    partition?: unknown;
    sort?: unknown;
    filter?: unknown;
  };
  if (request.sort !== undefined) {
    requireReadCondition(request.sort, `${prefix}.sort`);
  }
  if (request.filter !== undefined) {
    if (!Array.isArray(request.filter)) {
      throw new Error(`${prefix}.filter must be an array`);
    }
    for (const [index, cond] of request.filter.entries()) {
      requireReadCondition(cond, `${prefix}.filter[${index}]`);
    }
  }
}

function requireReadCondition(value: unknown, prefix: string): void {
  requirePlainObject(value, `${prefix} is required`);
  const condition = value as {
    attribute?: unknown;
    operator?: unknown;
    value?: unknown;
    values?: unknown;
  };
  if (typeof condition.attribute !== "string" || !condition.attribute) {
    throw new Error(`${prefix}.attribute is required`);
  }
  if (typeof condition.operator !== "string" || !condition.operator) {
    throw new Error(`${prefix}.operator is required`);
  }
  if (
    condition.value === undefined &&
    condition.values === undefined &&
    !readOperatorAllowsNoValue(condition.operator)
  ) {
    throw new Error(`${prefix}.value or ${prefix}.values is required`);
  }
}

function readOperatorAllowsNoValue(operator: unknown): boolean {
  if (typeof operator !== "string") return false;
  return [
    "exists",
    "attribute_exists",
    "not_exists",
    "attribute_not_exists",
  ].includes(operator.toLowerCase());
}

function requireTransitionActual(value: unknown, prefix: string): void {
  requirePlainObject(value, `${prefix} is required`);
  const actual = value as { model?: unknown; key?: unknown; set?: unknown };
  if (typeof actual.model !== "string" || actual.model.length === 0) {
    throw new Error(`${prefix}.model is required`);
  }
  requirePlainObject(actual.key, `${prefix}.key is required`);
  requirePlainObject(actual.set, `${prefix}.set is required`);
}

function requireTransitionEvent(value: unknown, prefix: string): void {
  requirePlainObject(value, `${prefix} is required`);
  const event = value as { model?: unknown; item?: unknown };
  if (typeof event.model !== "string" || event.model.length === 0) {
    throw new Error(`${prefix}.model is required`);
  }
  requirePlainObject(event.item, `${prefix}.item is required`);
}

function requirePlainObject(value: unknown, message: string): void {
  if (!isPlainObject(value) || Object.keys(value).length === 0) {
    throw new Error(message);
  }
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    return false;
  const proto = Object.getPrototypeOf(value);
  return proto === Object.prototype || proto === null;
}
