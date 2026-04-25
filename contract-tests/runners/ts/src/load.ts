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
  if (!Array.isArray(scenario.steps) || scenario.steps.length === 0) {
    throw new Error(`Scenario missing steps: ${filePath}`);
  }
  for (const [index, step] of scenario.steps.entries()) {
    const prefix = `${filePath} step ${index} ${step.op}`;
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
      case "transition_append_event":
        requireTransitionActual(step.actual, `${prefix}: actual`);
        requireTransitionEvent(step.event, `${prefix}: event`);
        break;
      case "validate_provenance":
        requirePlainObject(step.item, `${prefix}: item is required`);
        break;
      default:
        throw new Error(
          `${filePath} step ${index}: unsupported op ${String(step.op)}`,
        );
    }
  }
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
