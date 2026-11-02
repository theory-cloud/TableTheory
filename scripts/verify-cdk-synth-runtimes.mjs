#!/usr/bin/env node
// Purpose: after `cdk synth`, assert locally that every synthesized Lambda
// function resolves to a supported runtime. This reads cdk.out only: it never
// deploys, calls AWS, or mutates the synthesized stacks.
//
// The verdict sets are the runtime-deprecation gate's own, so a runtime admitted
// there is judged here exactly the same way. A function whose effective runtime
// is dynamic (a Ref object, an interpolation, or a non-string) fails closed: a
// synthesized template should already have resolved it.
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  DEPRECATED_LAMBDA_RUNTIMES,
  SUPPORTED_LAMBDA_RUNTIMES,
} from "./check-lambda-runtime-deprecations.mjs";

class CheckFailure extends Error {}

function fail(message) {
  throw new CheckFailure(message);
}

function isObject(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function evaluateTemplates(directory) {
  if (!fs.existsSync(directory) || !fs.statSync(directory).isDirectory()) {
    fail(`missing synthesized output directory ${directory}`);
  }
  const files = fs
    .readdirSync(directory)
    .filter((name) => name.endsWith(".template.json"))
    .sort();
  if (files.length === 0) fail(`no *.template.json under ${directory}`);

  const violations = [];
  let functions = 0;

  for (const file of files) {
    const absolute = path.join(directory, file);
    let document;
    try {
      document = JSON.parse(fs.readFileSync(absolute, "utf8"));
    } catch (err) {
      fail(`${file} is not valid JSON: ${err.message}`);
    }
    if (!isObject(document)) fail(`${file} is not a template object`);

    const resources = document.Resources;
    if (resources === undefined) continue;
    if (!isObject(resources)) fail(`${file} Resources is not an object`);

    const globalsRuntime = isObject(document.Globals) && isObject(document.Globals.Function)
      ? document.Globals.Function.Runtime
      : undefined;

    for (const [name, resource] of Object.entries(resources)) {
      if (!isObject(resource)) continue;
      if (resource.Type !== "AWS::Lambda::Function" && resource.Type !== "AWS::Serverless::Function") {
        continue;
      }

      const properties = isObject(resource.Properties) ? resource.Properties : {};
      const explicitRuntime = properties.Runtime;
      if (explicitRuntime === undefined && properties.PackageType === "Image") continue;

      const effectiveRuntime = explicitRuntime === undefined ? globalsRuntime : explicitRuntime;
      if (
        typeof effectiveRuntime !== "string" ||
        effectiveRuntime.trim() === "" ||
        effectiveRuntime.includes("${")
      ) {
        violations.push(`${file}:${name} effective runtime is not a readable string`);
        continue;
      }

      functions += 1;
      if (DEPRECATED_LAMBDA_RUNTIMES.includes(effectiveRuntime)) {
        violations.push(`${file}:${name} declares '${effectiveRuntime}', which AWS has deprecated`);
      } else if (!SUPPORTED_LAMBDA_RUNTIMES.includes(effectiveRuntime)) {
        violations.push(
          `${file}:${name} declares '${effectiveRuntime}', which is neither a deprecated runtime ` +
            "nor a supported pinned runtime",
        );
      }
    }
  }

  if (functions === 0) {
    fail(`no Lambda function runtime found across ${files.length} synthesized template(s)`);
  }

  return { files: files.length, functions, violations };
}

function writeTemplate(root, name, value) {
  fs.mkdirSync(root, { recursive: true });
  fs.writeFileSync(path.join(root, name), JSON.stringify(value));
}

function expectFailure(label, directory, expected) {
  try {
    evaluateTemplates(directory);
  } catch (err) {
    if (!(err instanceof CheckFailure) || !err.message.includes(expected)) {
      throw new CheckFailure(
        `self-test ${label}: expected failure containing ${JSON.stringify(expected)}, got ` +
          `${err.name}: ${err.message}`,
      );
    }
    return;
  }
  throw new CheckFailure(`self-test ${label}: expected a failure, but the assertion passed`);
}

function runSelfTest() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "tabletheory-cdk-synth-runtimes-"));
  try {
    const supported = path.join(root, "supported");
    writeTemplate(supported, "stack.template.json", {
      Resources: {
        Fn: { Type: "AWS::Lambda::Function", Properties: { Runtime: "provided.al2023" } },
      },
    });
    const supportedResult = evaluateTemplates(supported);
    if (supportedResult.violations.length > 0 || supportedResult.functions !== 1) {
      throw new CheckFailure("self-test supported template was not accepted");
    }

    const override = path.join(root, "override");
    writeTemplate(override, "stack.template.json", {
      Globals: { Function: { Runtime: "provided.al2023" } },
      Resources: {
        Fn: { Type: "AWS::Lambda::Function", Properties: { Runtime: "provided.al2" } },
      },
    });
    const overrideResult = evaluateTemplates(override);
    if (!overrideResult.violations.some((entry) => entry.includes("AWS has deprecated"))) {
      throw new CheckFailure("self-test deprecated resource override was not reported");
    }

    const dynamic = path.join(root, "dynamic");
    writeTemplate(dynamic, "stack.template.json", {
      Globals: { Function: { Runtime: "provided.al2023" } },
      Resources: {
        Good: { Type: "AWS::Lambda::Function", Properties: { Runtime: "provided.al2023" } },
        Fn: { Type: "AWS::Lambda::Function", Properties: { Runtime: { Ref: "RuntimeParameter" } } },
      },
    });
    const dynamicResult = evaluateTemplates(dynamic);
    if (!dynamicResult.violations.some((entry) => entry.includes("not a readable string"))) {
      throw new CheckFailure("self-test dynamic effective runtime was not reported");
    }

    const empty = path.join(root, "empty");
    writeTemplate(empty, "stack.template.json", { Resources: {} });
    expectFailure("empty template", empty, "no Lambda function runtime");

    console.log(
      "cdk-synth-runtimes self-test: PASS (supported template, deprecated override, dynamic override, empty template)",
    );
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
}

function main() {
  const args = process.argv.slice(2);
  if (args.length === 1 && args[0] === "--self-test") {
    runSelfTest();
    return;
  }
  if (args.length !== 1) {
    fail("usage: node scripts/verify-cdk-synth-runtimes.mjs <cdk.out-dir> | --self-test");
  }

  const result = evaluateTemplates(args[0]);
  if (result.violations.length > 0) {
    for (const violation of result.violations) {
      console.error(`cdk-synth-runtimes: ${violation}`);
    }
    fail(
      `${result.violations.length} synthesized Lambda runtime declaration(s) are deprecated or ` +
        `unmodelled (${result.functions} functions across ${result.files} template(s))`,
    );
  }
  console.log(
    `cdk-synth-runtimes: PASS (${result.functions} Lambda function(s) across ` +
      `${result.files} synthesized template(s); all effective runtimes supported)`,
  );
}

const invokedDirectly =
  process.argv[1] !== undefined && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (invokedDirectly) {
  try {
    main();
  } catch (err) {
    if (err instanceof CheckFailure) {
      console.error(`cdk-synth-runtimes: FAIL (${err.message})`);
      process.exit(1);
    }
    throw err;
  }
}

export { evaluateTemplates };
