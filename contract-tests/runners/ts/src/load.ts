import fs from "node:fs/promises";
import path from "node:path";
import YAML from "yaml";
import type { DmsDocument, DmsModel, Scenario } from "./types.js";

export async function loadModelsDir(modelsDir: string): Promise<Map<string, DmsModel>> {
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
  return scenario;
}

export async function loadScenariosDir(dir: string): Promise<Scenario[]> {
  const entries = await fs.readdir(dir, { withFileTypes: true });
  const files = entries
    .filter((e) => e.isFile() && (e.name.endsWith(".yml") || e.name.endsWith(".yaml")))
    .map((e) => path.join(dir, e.name))
    .sort();

  const scenarios: Scenario[] = [];
  for (const filePath of files) {
    scenarios.push(await loadScenarioFile(filePath));
  }
  return scenarios;
}

