// Purpose: fail when a TableTheory surface that declares an AWS Lambda runtime
// declares one AWS has deprecated.
//
// ===========================================================================
// Why this gate exists
// ===========================================================================
// AWS deprecates Lambda runtimes on a published schedule, and CDK reports the
// move as a synth-time WARNING. A warning is not a gate: it is easy to read past
// for years while every check stays green. The real deadline is not "the
// function stops working" but "you can no longer create a function on that
// runtime", which is a hard stop for a new stack, a new region, or a
// disaster-recovery rebuild. This gate turns the warning into a failure at PR
// time, before the declaration lands.
//
// ===========================================================================
// The deprecated set, and where its data comes from
// ===========================================================================
// DEPRECATED_LAMBDA_RUNTIMES below is the one place the deprecated set lives,
// and SUPPORTED_LAMBDA_RUNTIMES is the matching positive set. A declared
// runtime in neither set fails the gate, so admitting a runtime is a deliberate
// edit to this file rather than something an unmodelled value slips past.
//
// Every identifier below is cited against AWS's published runtime tables:
//
//   https://docs.aws.amazon.com/lambda/latest/dg/lambda-runtimes.html
//
// "Deprecated runtimes" (reached end of support) supplies the deprecated set:
//
//   nodejs            Node.js 0.10   deprecated 2016-08-30
//   nodejs4.3         Node.js 4.3    deprecated 2020-03-05
//   nodejs4.3-edge    Node.js 4.3e   deprecated 2020-03-05
//   nodejs6.10        Node.js 6.10   deprecated 2019-08-12
//   nodejs8.10        Node.js 8.10   deprecated 2020-03-06
//   nodejs10.x        Node.js 10     deprecated 2021-07-30
//   nodejs12.x        Node.js 12     deprecated 2023-03-31
//   nodejs14.x        Node.js 14     deprecated 2023-12-04
//   nodejs16.x        Node.js 16     deprecated 2024-06-12
//   nodejs18.x        Node.js 18     deprecated 2025-09-01
//   nodejs20.x        Node.js 20     deprecated 2026-04-30
//   python2.7         Python 2.7     deprecated 2021-07-15
//   python3.6         Python 3.6     deprecated 2022-07-18
//   python3.7         Python 3.7     deprecated 2023-12-04
//   python3.8         Python 3.8     deprecated 2024-10-14
//   python3.9         Python 3.9     deprecated 2025-12-15
//   provided          OS-only (AL1)  deprecated 2024-01-08
//   provided.al2      OS-only (AL2)  deprecated 2026-07-31
//
// "Supported runtimes" supplies the supported set for the same three families,
// each with its AWS-projected deprecation date so a later editor can see how
// much runway each entry has left:
//
//   nodejs22.x        projected 2027-04-30
//   nodejs24.x        projected 2028-04-30
//   python3.10        projected 2026-10-31
//   python3.11        projected 2027-06-30
//   python3.12        projected 2028-10-31
//   python3.13        projected 2029-06-30
//   python3.14        projected 2029-06-30
//   provided.al2023   projected 2029-06-30
//
// Two families AWS lists as preview rather than generally available
// (nodejs26.x and python3.15, both "Not scheduled") are deliberately NOT in the
// supported set. A preview runtime is not covered by the Lambda SLA, so a
// declaration of one fails closed as unmodelled and has to be admitted here
// deliberately once AWS makes it generally available.
//
// TableTheory ships non-nodejs runtimes - python3.14 and provided.al2023 are
// declared in examples/cdk-multilang today - so the model is extended past
// FaceTheory's nodejs-only set. The other AWS families (ruby, java, dotnet,
// go) are visible to the classifier but carry no data here, so a declaration
// in one of them fails closed rather than being skipped.
//
// The family list is closed, and that is a limit rather than a blanket
// fail-closed guarantee. A quoted literal whose family is not in the list is not
// runtime-shaped at all, so `lambda.Runtime.fromString('rust1.0')` declares
// nothing; and the shape rule requires a version component, so
// `lambda.Runtime.fromString('provided')` declares nothing either even though
// `provided` IS in the deprecated set. As with the receiver limits below, either
// one fails closed when it is its surface's only declaration and is invisible
// when the surface declares something else. Scoping the quoted-literal rule to
// `fromString(...)` arguments would close both; that change belongs to the
// cross-repo follow-up wave carrying the same defect in FaceTheory's checker,
// and is deliberately not made here.
//
// ===========================================================================
// Scope
// ===========================================================================
// SCANNED_SURFACES are the surfaces that decide the runtime of the Lambda
// functions TableTheory ships:
//
//   examples/cdk-multilang/lib/multilang-demo-stack.ts
//   examples/cdk-multilang/lib/tabletheory-ttl-archive.ts
//
// Coverage is enforced rather than assumed. SCAN_ROOTS is walked and every file
// under it that declares a Lambda runtime must be a declared surface; a file
// that declares one without being listed fails the gate, so a new CDK stack
// cannot arrive declaring a runtime nobody judges. Symmetrically, a declared
// surface must exist and must still declare at least one modelled runtime, so
// the gate cannot be neutered by deleting a line or by switching a surface to a
// runtime value the model cannot read.
//
// "Declares a runtime" means the same thing to the coverage walk and to the
// per-surface scan, and it covers the receiver forms a real surface uses:
// `lambda.Runtime.X`, a named import's `Runtime.X`, and either of those aliased
// to a local name.
//
// A form the receiver model cannot read declares nothing, and what that costs
// depends on whether it was the surface's LAST modelled declaration. Hiding the
// only one - behind an index, a lookup table, or a helper - leaves the surface
// with no modelled runtime, which fails closed. Hiding one of several does not:
// the remaining declarations still satisfy the coverage walk, so this checker
// passes, and the obfuscated runtime is caught instead by the policy test's
// `declarations 4` count pin over the real surfaces. The multi-declaration case
// is therefore a caught regression rather than a silent one, but it is caught by
// that pin, not by the coverage walk.
//
// Three receiver forms are measured as declaring nothing, and are known limits
// of this classifier rather than waivers:
//
//   Runtime['PYTHON_3_8']         bracket access: not a `.` member access
//   const S = R                   a second hop: an alias of an alias binds to R
//                                 rather than to `lambda.Runtime`, so S never
//                                 becomes a receiver
//   const { Runtime: RT } = ...   a renamed destructure: only the
//                                 `import { Runtime as R }` form registers an
//                                 alias
//
// Plain `const { Runtime } = lambda` IS caught, because the bare `Runtime`
// receiver is fixed rather than alias-derived.
//
// A member access that is followed by a property read is the same declaration,
// not a second one: `lambda.Runtime.PROVIDED_AL2023.bundlingImage` reads the
// bundling image of the runtime the surrounding `runtime:` line already
// declared. Declarations are therefore de-duplicated per surface by the
// runtime they resolve to (falling back to the member access as written when
// the runtime cannot be resolved), so counting a surface's declarations counts
// runtimes, not occurrences. This is a deliberate divergence from FaceTheory's
// checker, which counts the property read separately.
//
// Fail-closed cases: a surface that is missing, unreadable, or declares no
// modelled runtime; a runtime literal in a modelled shape whose family or value
// is not modelled; a declaration that names an unpinned moving alias rather
// than a pinned runtime; and any file inside SCAN_ROOTS that declares a Lambda
// runtime without being a declared surface.
//
// The scope is owned by this checker and there is no allowlist, no waiver flag,
// and no exception list. EXPECTED_SCANNED_SURFACES and EXPECTED_SCAN_ROOTS are
// independent copies of the scope, so the self-test fails when either is
// narrowed. `--self-test` is the only argument, and it runs the classifier
// self-test plus synthetic surfaces driven through the real read path before
// the real scan. The checker reads no environment variable.
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

// === the deprecated set, explicit in one place =============================
// Sourced from the "Deprecated runtimes" table at
// https://docs.aws.amazon.com/lambda/latest/dg/lambda-runtimes.html
const DEPRECATED_LAMBDA_RUNTIMES = [
  // nodejs family
  "nodejs",
  "nodejs4.3",
  "nodejs4.3-edge",
  "nodejs6.10",
  "nodejs8.10",
  "nodejs10.x",
  "nodejs12.x",
  "nodejs14.x",
  "nodejs16.x",
  "nodejs18.x",
  "nodejs20.x",
  // python family
  "python2.7",
  "python3.6",
  "python3.7",
  "python3.8",
  "python3.9",
  // provided (OS-only) family
  "provided",
  "provided.al2",
];

// Sourced from the "Supported runtimes" table at the same page.
const SUPPORTED_LAMBDA_RUNTIMES = [
  // nodejs family
  "nodejs22.x",
  "nodejs24.x",
  // python family
  "python3.10",
  "python3.11",
  "python3.12",
  "python3.13",
  "python3.14",
  // provided (OS-only) family
  "provided.al2023",
];

const SCANNED_SURFACES = [
  "examples/cdk-multilang/lib/multilang-demo-stack.ts",
  "examples/cdk-multilang/lib/tabletheory-ttl-archive.ts",
];

// The scanned set, restated independently of SCANNED_SURFACES so the self-test
// catches a scan scope that was narrowed instead of widened.
const EXPECTED_SCANNED_SURFACES = [
  "examples/cdk-multilang/lib/multilang-demo-stack.ts",
  "examples/cdk-multilang/lib/tabletheory-ttl-archive.ts",
];

const SCAN_ROOTS = ["examples/cdk-multilang/lib"];

const EXPECTED_SCAN_ROOTS = ["examples/cdk-multilang/lib"];

// Directory names the coverage walk does not descend into: dependency, build,
// coverage, and vendored trees that hold no declaration of ours.
const SKIPPED_DIRECTORY_NAMES = new Set([
  "node_modules",
  "dist",
  "coverage",
  "cdk.out",
  ".cdk.staging",
  "vendor",
]);

// CDK's enum names for the runtimes this gate models, read from the pinned
// aws-cdk-lib 2.270.0 Runtime class in examples/cdk-multilang. An enum name that
// is absent here fails the gate rather than being skipped, so a runtime has to
// be added deliberately.
//
// The map is deliberately narrower than the CDK Runtime class: it covers the
// nodejs, python, and provided families TableTheory deploys. RUBY_3_2,
// JAVA_17, GO_1_X and the rest fail closed as unmodelled enums, which is the
// "declarations are never skipped" contract rather than an oversight.
//
// CDK 2.270.0 has no NODEJS_4_3_EDGE member, so the AWS identifier
// `nodejs4.3-edge` is reachable only as a literal. That is why the self-test
// asserts the map is a subset of the two sets rather than that they are equal.
const LAMBDA_RUNTIME_ENUMS = new Map([
  // nodejs family
  ["NODEJS", "nodejs"],
  ["NODEJS_4_3", "nodejs4.3"],
  ["NODEJS_6_10", "nodejs6.10"],
  ["NODEJS_8_10", "nodejs8.10"],
  ["NODEJS_10_X", "nodejs10.x"],
  ["NODEJS_12_X", "nodejs12.x"],
  ["NODEJS_14_X", "nodejs14.x"],
  ["NODEJS_16_X", "nodejs16.x"],
  ["NODEJS_18_X", "nodejs18.x"],
  ["NODEJS_20_X", "nodejs20.x"],
  ["NODEJS_22_X", "nodejs22.x"],
  ["NODEJS_24_X", "nodejs24.x"],
  // python family
  ["PYTHON_2_7", "python2.7"],
  ["PYTHON_3_6", "python3.6"],
  ["PYTHON_3_7", "python3.7"],
  ["PYTHON_3_8", "python3.8"],
  ["PYTHON_3_9", "python3.9"],
  ["PYTHON_3_10", "python3.10"],
  ["PYTHON_3_11", "python3.11"],
  ["PYTHON_3_12", "python3.12"],
  ["PYTHON_3_13", "python3.13"],
  ["PYTHON_3_14", "python3.14"],
  // provided (OS-only) family
  ["PROVIDED", "provided"],
  ["PROVIDED_AL2", "provided.al2"],
  ["PROVIDED_AL2023", "provided.al2023"],
]);

// `NODEJS_LATEST` is not a pinned runtime: it is an alias that follows the CDK
// version, so a dependency bump would change what these stacks deploy without a
// line of stack code changing. That is the opposite of the pinned-runtime
// contract, so it fails on its own reason rather than being reported as
// deprecated.
const UNPINNED_RUNTIME_ALIAS_ENUMS = new Set(["NODEJS_LATEST"]);

// AWS's runtime families. The classifier is not an alternation of these names:
// the list is what makes a runtime-shaped quoted literal a declaration at all,
// so a declaration in a family with no data below still reaches the classifier
// and fails closed instead of being invisible - `fromString('ruby3.2')` is a
// violation, not a skip. Keeping the list closed is deliberate, because loosening
// it further would turn unrelated identifiers such as the `target: 'node24'`
// bundling option into declarations; the measured cost is that a family outside
// the list declares nothing, so `fromString('rust1.0')` is invisible rather than
// fail-closed. The header states both that limit and the version-component one.
const LAMBDA_RUNTIME_FAMILIES = [
  "nodejs",
  "python",
  "ruby",
  "java",
  "dotnet",
  "dotnetcore",
  "go",
  "provided",
];

const UNMODELLED_DECLARATION_HINT =
  "add the runtime deliberately to DEPRECATED_LAMBDA_RUNTIMES or SUPPORTED_LAMBDA_RUNTIMES " +
  "in scripts/check-lambda-runtime-deprecations.mjs - declarations are never skipped";

class GateFailure extends Error {}

function fail(message) {
  console.error(`lambda-runtime-deprecations: FAIL (${message})`);
  process.exit(1);
}

// === declaration forms =====================================================

// Constant-style members only, so `lambda.Runtime.NODEJS_20_X` is a declaration
// while `lambda.Runtime.fromString(...)` is not: a method's argument is judged
// by the literal rule below, and an unmodelled dynamic runtime leaves the
// surface without a declaration, which fails closed on its own.
//
// The receiver is read in three forms, because requiring the literal text
// `lambda.Runtime.` left the other two declaring nothing at all:
//
//   lambda.Runtime.NODEJS_20_X   a `lambda` namespace import's member
//   Runtime.NODEJS_20_X          a named import: `import { Runtime } from ...`
//   R.NODEJS_20_X                either of the above aliased to a local name:
//                                `const R = lambda.Runtime`, or
//                                `import { Runtime as R } from ...`
//
// A new undeclared surface written in either of the last two idioms would be
// invisible to the classifier and to the coverage walk alike, so it would pass
// the gate without ever being judged. The receiver list is therefore built per
// surface: the two fixed receivers plus whatever aliases that surface binds.
const RUNTIME_NAMESPACE_RECEIVERS = ["lambda\\.Runtime", "Runtime"];

// An alias is a binding, never a declaration: an enum member still has to
// follow it. A surface that binds the namespace and then hides the member ends
// up with no readable declaration, which is the fail-closed outcome this gate
// wants rather than a reason to widen further.
const RUNTIME_NAMESPACE_ALIAS_RES = [
  // const R = lambda.Runtime   /   let R = Runtime
  /\b(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:lambda\.Runtime|Runtime)\b(?!\s*\.)/g,
  // import { Runtime as R } from 'aws-cdk-lib/aws-lambda'
  /\bimport\s*\{[^}]*?\bRuntime\s+as\s+([A-Za-z_$][A-Za-z0-9_$]*)[^}]*?\}\s*from\s*["'][^"']*aws-lambda["']/g,
];

// Every runtime enum this gate models is SCREAMING_CASE, and that is what keeps
// the widened receiver prefix from swallowing unrelated `Runtime.` namespaces
// and from reading a method or property name as a runtime: `Runtime.fromString`
// and `Runtime.PROVIDED_AL2023.bundlingImage` name methods and properties in
// lowerCamelCase, so neither matches. Node's inspector domain spells its
// members `Runtime.ScriptId` and `Runtime.StackTrace`, which do not match
// either.
const ENUM_MEMBER_NAME = "([A-Z][A-Z0-9_]*)";

function escapeRegExp(text) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// The enum-member pattern for one surface: the fixed receivers first, so
// `lambda.Runtime.NODEJS_20_X` is consumed whole rather than re-matching as the
// bare `Runtime.NODEJS_20_X` inside it, then that surface's aliases.
function enumDeclarationPattern(text) {
  const aliases = new Set();
  for (const pattern of RUNTIME_NAMESPACE_ALIAS_RES) {
    for (const match of text.matchAll(pattern)) aliases.add(match[1]);
  }
  const receivers = [...RUNTIME_NAMESPACE_RECEIVERS, ...[...aliases].sort().map(escapeRegExp)];
  return new RegExp(`\\b(${receivers.join("|")})\\.${ENUM_MEMBER_NAME}\\b`, "g");
}

// Quoted strings that could be a runtime identifier. Deliberately loose - the
// predicate below decides, so a candidate that is not a runtime is simply not a
// declaration - but it must not be an alternation of family names, or an
// unfamiliar family would be invisible instead of failing closed.
const QUOTED_LITERAL_RE = /["'`]([a-z][a-z0-9._-]{2,})["'`]/g;

// The identifier a quoted string names, or null when it is not runtime-shaped.
// A runtime identifier is a known family followed by a version component, so
// `python3.14` and `provided.al2023` are declarations while `node_modules`,
// `assets`, `go.mod`, and the `node24` esbuild bundling target are not.
function runtimeIdentifierFromLiteral(value) {
  const match = /^([a-z][a-z0-9-]*?)(\.al[0-9]+|[0-9][0-9A-Za-z._-]*)$/.exec(value);
  if (match === null) return null;
  return LAMBDA_RUNTIME_FAMILIES.includes(match[1]) ? value : null;
}

function lineOf(text, index) {
  let line = 1;
  for (let cursor = 0; cursor < index; cursor += 1) {
    if (text[cursor] === "\n") line += 1;
  }
  return line;
}

// Collects every runtime declaration in a surface's text, in source order, as
// { line, form, identifier, enumName }. `identifier` is null when the form
// could not be resolved to a runtime, and `enumName` is null for literal forms.
// `form` is the receiver exactly as written, so a violation reports the idiom
// the surface actually used rather than the one the gate prefers.
//
// Declarations are de-duplicated by the runtime they resolve to, falling back
// to the member access as written when there is no runtime to resolve. That is
// what keeps `lambda.Runtime.PROVIDED_AL2023.bundlingImage` from being counted
// as a second `lambda.Runtime.PROVIDED_AL2023` declaration: both resolve to
// provided.al2023. Two genuinely different declarations are never merged,
// because two different runtimes or two different unreadable members resolve to
// two different keys.
function collectDeclarations(text) {
  const byKey = new Map();

  const record = (index, form, enumName, identifier) => {
    const key = identifier === null ? `form:${form}` : `runtime:${identifier}`;
    if (byKey.has(key)) return;
    byKey.set(key, { index, line: lineOf(text, index), form, enumName, identifier });
  };

  for (const match of text.matchAll(enumDeclarationPattern(text))) {
    const receiver = match[1];
    const enumName = match[2];
    record(match.index, `${receiver}.${enumName}`, enumName, LAMBDA_RUNTIME_ENUMS.get(enumName) ?? null);
  }
  for (const match of text.matchAll(QUOTED_LITERAL_RE)) {
    const identifier = runtimeIdentifierFromLiteral(match[1]);
    if (identifier === null) continue;
    record(match.index, `"${match[1]}"`, null, identifier);
  }

  return [...byKey.values()].sort((left, right) => left.index - right.index);
}

// === classification ========================================================

// Classifies one declaration. Returns a violation object, or null when the
// declaration is a supported pinned runtime.
function classifyDeclaration(declaration) {
  if (declaration.enumName !== null && UNPINNED_RUNTIME_ALIAS_ENUMS.has(declaration.enumName)) {
    return {
      kind: "unpinned-alias",
      line: declaration.line,
      form: declaration.form,
      detail:
        `names the moving alias lambda.Runtime.${declaration.enumName}, which follows the CDK ` +
        `version instead of pinning a runtime`,
    };
  }
  if (declaration.identifier === null) {
    return {
      kind: "unmodelled-enum",
      line: declaration.line,
      form: declaration.form,
      detail:
        `names the runtime enum ${declaration.enumName}, which ` +
        `scripts/check-lambda-runtime-deprecations.mjs does not model; ${UNMODELLED_DECLARATION_HINT}`,
    };
  }
  if (DEPRECATED_LAMBDA_RUNTIMES.includes(declaration.identifier)) {
    return {
      kind: "deprecated",
      line: declaration.line,
      form: declaration.form,
      detail: `declares '${declaration.identifier}', which AWS has deprecated`,
    };
  }
  if (SUPPORTED_LAMBDA_RUNTIMES.includes(declaration.identifier)) return null;
  return {
    kind: "unmodelled-runtime",
    line: declaration.line,
    form: declaration.form,
    detail:
      `declares '${declaration.identifier}', which is neither a deprecated runtime nor a ` +
      `supported pinned runtime; ${UNMODELLED_DECLARATION_HINT}`,
  };
}

// === scan ==================================================================

function resolveSurface(label) {
  return path.isAbsolute(label) ? label : path.join(repositoryRoot, label);
}

function scanSurface(label) {
  const absolute = resolveSurface(label);
  let text;
  try {
    text = fs.readFileSync(absolute, "utf8");
  } catch (err) {
    throw new GateFailure(`could not read scanned surface ${label}: ${err.message}`);
  }
  const declarations = collectDeclarations(text);
  if (declarations.length === 0) {
    throw new GateFailure(
      `${label} declares no modelled Lambda runtime; a scanned surface must still declare one, ` +
        `so the gate cannot be neutered by deleting or obfuscating its runtime`,
    );
  }
  return {
    label,
    declarationCount: declarations.length,
    declarations,
    violations: declarations
      .map(classifyDeclaration)
      .filter((violation) => violation !== null)
      .map((violation) => ({ ...violation, label })),
  };
}

// Every file inside SCAN_ROOTS that declares a Lambda runtime, whether or not it
// is a declared surface. Used to fail closed on an unmodelled surface.
function collectDeclarationSurfaces() {
  const found = new Set();

  const visit = (absolute) => {
    const entries = fs.readdirSync(absolute, { withFileTypes: true });
    for (const entry of entries) {
      const child = path.join(absolute, entry.name);
      if (entry.isDirectory()) {
        if (SKIPPED_DIRECTORY_NAMES.has(entry.name)) continue;
        visit(child);
        continue;
      }
      if (!entry.isFile()) continue;
      let text;
      try {
        text = fs.readFileSync(child, "utf8");
      } catch {
        continue;
      }
      if (collectDeclarations(text).length > 0) {
        found.add(path.relative(repositoryRoot, child));
      }
    }
  };

  for (const root of SCAN_ROOTS) {
    const absolute = path.join(repositoryRoot, root);
    if (!fs.existsSync(absolute)) {
      throw new GateFailure(`scan root ${root} is missing; the coverage scope cannot be walked`);
    }
    visit(absolute);
  }

  return [...found].sort();
}

function describeViolation(violation) {
  return `${violation.label}:${violation.line} ${violation.form} ${violation.detail}`;
}

// === self-test =============================================================

// [text, expected violation kinds] - driven through the real read path.
const CLASSIFIER_CASES = [
  {
    name: "supported-pinned-nodejs-runtime",
    text: "const fn = new NodejsFunction(this, 'Fn', {\n  runtime: lambda.Runtime.NODEJS_24_X,\n});\n",
    expected: [],
  },
  {
    name: "supported-floor-nodejs-runtime",
    text: "runtime: lambda.Runtime.NODEJS_22_X,\n",
    expected: [],
  },
  {
    name: "deprecated-nodejs-runtime",
    text: "runtime: lambda.Runtime.NODEJS_20_X,\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-nodejs-runtime-older-than-the-minimum",
    text: "runtime: lambda.Runtime.NODEJS_4_3,\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-nodejs-literal",
    text: "runtime: lambda.Runtime.fromString('nodejs18.x'),\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-nodejs-literal",
    text: "runtime: lambda.Runtime.fromString('nodejs24.x'),\n",
    expected: [],
  },
  // --- the non-nodejs families TableTheory declares -------------------------
  {
    name: "supported-python-runtime",
    text: "runtime: lambda.Runtime.PYTHON_3_14,\n",
    expected: [],
  },
  {
    name: "deprecated-python-runtime",
    text: "runtime: lambda.Runtime.PYTHON_3_8,\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-python-runtime-literal",
    text: "runtime: lambda.Runtime.fromString('python3.9'),\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-python-runtime-literal",
    text: "runtime: lambda.Runtime.fromString('python3.13'),\n",
    expected: [],
  },
  {
    name: "supported-provided-al2023-runtime",
    text: "runtime: lambda.Runtime.PROVIDED_AL2023,\n",
    expected: [],
  },
  {
    name: "deprecated-provided-al2-runtime",
    text: "runtime: lambda.Runtime.PROVIDED_AL2,\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-provided-runtime-literal",
    text: "runtime: lambda.Runtime.fromString('provided.al2'),\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-provided-al2023-runtime-literal",
    text: "runtime: lambda.Runtime.fromString('provided.al2023'),\n",
    expected: [],
  },
  // A family the gate has no data for is a declaration that fails closed, not
  // one that is invisible.
  {
    name: "unmodelled-family-literal",
    text: "runtime: lambda.Runtime.fromString('ruby3.2'),\n",
    expected: ["unmodelled-runtime"],
  },
  {
    name: "unmodelled-family-enum",
    text: "runtime: lambda.Runtime.RUBY_3_2,\n",
    expected: ["unmodelled-enum"],
  },
  // A runtime AWS lists as preview is not in the supported set, so it fails
  // closed until an editor admits it deliberately.
  {
    name: "preview-runtime-literal-fails-closed",
    text: "runtime: lambda.Runtime.fromString('nodejs26.x'),\n",
    expected: ["unmodelled-runtime"],
  },
  // --- property reads on a runtime are not a second declaration -------------
  {
    name: "bundling-image-read-is-not-a-second-declaration",
    text: "runtime: lambda.Runtime.PROVIDED_AL2023,\ncode: lambda.Code.fromAsset(dir, {\n  bundling: { image: lambda.Runtime.PROVIDED_AL2023.bundlingImage },\n}),\n",
    expected: [],
    expectedDeclarations: ["provided.al2023"],
  },
  {
    name: "bundling-image-read-does-not-hide-a-deprecated-declaration",
    text: "runtime: lambda.Runtime.NODEJS_18_X,\ncode: lambda.Code.fromAsset(dir, {\n  bundling: { image: lambda.Runtime.NODEJS_18_X.bundlingImage },\n}),\n",
    expected: ["deprecated"],
    expectedDeclarations: ["nodejs18.x"],
  },
  {
    name: "unpinned-moving-alias",
    text: "runtime: lambda.Runtime.NODEJS_LATEST,\n",
    expected: ["unpinned-alias"],
  },
  {
    name: "every-declaration-is-judged",
    text: "runtime: lambda.Runtime.NODEJS_24_X,\nruntime: lambda.Runtime.NODEJS_20_X,\nruntime: lambda.Runtime.PYTHON_3_9,\n",
    expected: ["deprecated", "deprecated"],
  },
  {
    name: "quoted-strings-that-are-not-runtimes",
    text: 'runtime: lambda.Runtime.NODEJS_24_X,\nconst a = "node_modules";\nconst b = "assets";\nconst c = "go.mod";\nconst d = "vite";\nconst e = "1.2.3";\nconst f = "node24";\n',
    expected: [],
  },
  {
    name: "deprecated-runtime-through-a-named-import",
    text: "import { Runtime } from 'aws-cdk-lib/aws-lambda';\n\nnew lambda.Function(this, 'Fn', {\n  runtime: Runtime.NODEJS_20_X,\n});\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-runtime-through-a-named-import",
    text: "import { Runtime } from 'aws-cdk-lib/aws-lambda';\nruntime: Runtime.NODEJS_24_X,\n",
    expected: [],
  },
  {
    name: "deprecated-python-runtime-through-a-named-import",
    text: "import { Runtime } from 'aws-cdk-lib/aws-lambda';\nruntime: Runtime.PYTHON_3_8,\n",
    expected: ["deprecated"],
  },
  {
    name: "deprecated-runtime-through-an-aliased-namespace",
    text: "import * as lambda from 'aws-cdk-lib/aws-lambda';\nconst R = lambda.Runtime;\nruntime: R.NODEJS_20_X,\n",
    expected: ["deprecated"],
  },
  {
    name: "supported-runtime-through-an-aliased-namespace",
    text: "import * as lambda from 'aws-cdk-lib/aws-lambda';\nconst R = lambda.Runtime;\nruntime: R.PROVIDED_AL2023,\n",
    expected: [],
  },
  {
    name: "deprecated-runtime-through-a-renamed-named-import",
    text: "import { Runtime as R } from 'aws-cdk-lib/aws-lambda';\nruntime: R.NODEJS_18_X,\n",
    expected: ["deprecated"],
  },
];

const FAIL_CLOSED_CASES = [
  {
    name: "surface-without-a-declaration",
    text: "export const nothing = 1;\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  {
    name: "surface-of-only-quoted-non-runtimes",
    text: 'const a = "node_modules";\nconst b = "1.2.3";\n',
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  // Binding the namespace and then hiding the member is not a way to declare
  // nothing and still pass: the surface has no readable declaration, so it fails
  // closed on the same rule that catches an obfuscated surface.
  {
    name: "named-import-with-an-unreadable-member",
    text: "import { Runtime } from 'aws-cdk-lib/aws-lambda';\nruntime: Runtime[legacyRuntimeName],\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  {
    name: "aliased-namespace-with-an-unreadable-member",
    text: "import * as lambda from 'aws-cdk-lib/aws-lambda';\nconst R = lambda.Runtime;\nruntime: R[process.env.TABLETHEORY_RUNTIME],\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  // A method call is not a member declaration: `R.fromString(runtimeName)` names
  // `fromString`, not a runtime, and the argument is not a literal, so the
  // surface ends up with no readable declaration and fails closed.
  {
    name: "aliased-namespace-calling-fromString-with-a-variable",
    text: "const R = lambda.Runtime;\nruntime: R.fromString(runtimeName),\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
  {
    name: "dynamic-fromString-with-no-literal",
    text: "runtime: lambda.Runtime.fromString(process.env.LAMBDA_RUNTIME),\n",
    expectFailureReason: "declares no modelled Lambda runtime",
  },
];

function selfTestViolationKinds(outcome) {
  return outcome.violations.map((violation) => violation.kind);
}

function writeSyntheticSurface(tempDir, scenario) {
  const file = path.join(tempDir, scenario.name, "surface.ts");
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, scenario.text);
  return file;
}

function runSelfTest() {
  const failures = [];
  let classifierCases = 0;

  const scopeCases = 2;
  const scopeMatches =
    SCANNED_SURFACES.length === EXPECTED_SCANNED_SURFACES.length &&
    SCANNED_SURFACES.every((label, index) => label === EXPECTED_SCANNED_SURFACES[index]);
  if (!scopeMatches) {
    failures.push(
      `scanned surface set must be exactly ${JSON.stringify(EXPECTED_SCANNED_SURFACES)}, ` +
        `got ${JSON.stringify(SCANNED_SURFACES)}`,
    );
  }
  console.log(
    `  self-test scan scope: ${scopeMatches ? `surfaces ${SCANNED_SURFACES.length}` : "NOT THE EXPECTED SET"}`,
  );

  const rootMatches =
    SCAN_ROOTS.length === EXPECTED_SCAN_ROOTS.length &&
    SCAN_ROOTS.every((root, index) => root === EXPECTED_SCAN_ROOTS[index]);
  if (!rootMatches) {
    failures.push(
      `coverage scan roots must be exactly ${JSON.stringify(EXPECTED_SCAN_ROOTS)}, ` +
        `got ${JSON.stringify(SCAN_ROOTS)}`,
    );
  }
  console.log(
    `  self-test coverage scope: ${rootMatches ? `roots ${SCAN_ROOTS.length}` : "NOT THE EXPECTED SET"}`,
  );

  // The declared surface list and the coverage walk must agree, or the gate has
  // an unmodelled surface before it even starts scanning.
  try {
    const discovered = collectDeclarationSurfaces();
    const expected = [...SCANNED_SURFACES].sort();
    if (discovered.join("\n") !== expected.join("\n")) {
      failures.push(
        `coverage walk under ${JSON.stringify(SCAN_ROOTS)} must find exactly the declared surfaces; ` +
          `found ${JSON.stringify(discovered)}`,
      );
    }
    console.log(
      `  self-test coverage walk: ${discovered.join("\n") === expected.join("\n") ? `${discovered.length} surfaces` : "NOT THE DECLARED SET"}`,
    );
  } catch (err) {
    failures.push(`coverage walk threw ${err.message}`);
    console.log("  self-test coverage walk: UNEXPECTED FAILURE");
  }

  // The two classification sets and the enum map cannot drift apart. Every enum
  // name the map can produce must land in exactly one set, so a runtime cannot
  // be named by the map without carrying a verdict. The reverse direction is
  // deliberately not required: `nodejs4.3-edge` is an AWS runtime CDK has no
  // enum member for, and it is still reachable as a literal.
  for (const [enumName, identifier] of LAMBDA_RUNTIME_ENUMS) {
    const deprecated = DEPRECATED_LAMBDA_RUNTIMES.includes(identifier);
    const supported = SUPPORTED_LAMBDA_RUNTIMES.includes(identifier);
    if (deprecated === supported) {
      failures.push(
        `enum ${enumName} names ${identifier}, which is ` +
          `${deprecated ? "both deprecated and supported" : "in neither DEPRECATED_LAMBDA_RUNTIMES nor SUPPORTED_LAMBDA_RUNTIMES"}`,
      );
    }
  }
  for (const identifier of DEPRECATED_LAMBDA_RUNTIMES) {
    if (SUPPORTED_LAMBDA_RUNTIMES.includes(identifier)) {
      failures.push(`identifier ${identifier} is in both the deprecated and the supported set`);
    }
  }

  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "tabletheory-lambda-runtimes-"));
  try {
    for (const scenario of [...CLASSIFIER_CASES, ...FAIL_CLOSED_CASES]) {
      classifierCases += 1;
      const file = writeSyntheticSurface(tempDir, scenario);
      let outcome;
      try {
        outcome = scanSurface(file);
      } catch (err) {
        if (scenario.expectFailureReason === undefined) {
          failures.push(`synthetic surface ${scenario.name} threw ${err.message}`);
          console.log(`  self-test synthetic ${scenario.name}: UNEXPECTED FAILURE`);
          continue;
        }
        if (!(err instanceof GateFailure) || !err.message.includes(scenario.expectFailureReason)) {
          failures.push(
            `synthetic surface ${scenario.name} must fail closed with ` +
              `${JSON.stringify(scenario.expectFailureReason)}, got ${err.name}: ${err.message}`,
          );
          console.log(`  self-test synthetic ${scenario.name}: FAIL CLOSED FOR THE WRONG REASON`);
          continue;
        }
        console.log(
          `  self-test synthetic ${scenario.name}: FAIL CLOSED (${scenario.expectFailureReason})`,
        );
        continue;
      }
      if (scenario.expectFailureReason !== undefined) {
        failures.push(
          `synthetic surface ${scenario.name} must fail closed with ` +
            `${JSON.stringify(scenario.expectFailureReason)}, got ${JSON.stringify(selfTestViolationKinds(outcome))}`,
        );
        console.log(`  self-test synthetic ${scenario.name}: PASSED OPEN`);
        continue;
      }
      if (scenario.expectedDeclarations !== undefined) {
        const actualDeclarations = outcome.declarations.map(
          (declaration) => declaration.identifier ?? declaration.form,
        );
        const declarationsMatch =
          actualDeclarations.length === scenario.expectedDeclarations.length &&
          actualDeclarations.every((value, index) => value === scenario.expectedDeclarations[index]);
        if (!declarationsMatch) {
          failures.push(
            `synthetic surface ${scenario.name} expected declarations ` +
              `${JSON.stringify(scenario.expectedDeclarations)}, got ${JSON.stringify(actualDeclarations)}`,
          );
        }
      }
      const expected = scenario.expected;
      const actual = selfTestViolationKinds(outcome);
      const matched = actual.length === expected.length && actual.every((value, i) => value === expected[i]);
      if (!matched) {
        failures.push(
          `synthetic surface ${scenario.name} expected violations ${JSON.stringify(expected)}, ` +
            `got ${JSON.stringify(actual)}`,
        );
      }
      console.log(
        `  self-test synthetic ${scenario.name}: ${actual.length === 0 ? "PASS" : `FAIL ${actual.join("; ")}`}`,
      );
    }
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }

  const missingSurface = path.join(tempDir, "absent", "surface.ts");
  classifierCases += 1;
  try {
    scanSurface(missingSurface);
    failures.push("a missing scanned surface must fail closed, but the scan succeeded");
    console.log("  self-test missing surface: PASSED OPEN");
  } catch (err) {
    if (!(err instanceof GateFailure) || !err.message.includes("could not read scanned surface")) {
      failures.push(`missing scanned surface threw ${err.name}: ${err.message}`);
      console.log("  self-test missing surface: FAIL CLOSED FOR THE WRONG REASON");
    } else {
      console.log("  self-test missing surface: FAIL CLOSED (could not read scanned surface)");
    }
  }

  if (failures.length > 0) {
    for (const failure of failures) console.error(`  self-test: ${failure}`);
    throw new GateFailure(
      `self-test failed (${failures.length} failures across ${classifierCases} synthetic surfaces ` +
        `plus ${scopeCases} scope cases)`,
    );
  }

  return { classifierCases, scopeCases };
}

// === entry point ===========================================================

function main() {
  const args = process.argv.slice(2);
  const unsupported = args.filter((arg) => arg !== "--self-test");
  if (unsupported.length > 0) {
    fail(`unsupported arguments ${unsupported.join(" ")} (this gate takes no arguments)`);
  }
  const selfTest = args.includes("--self-test");
  // The self-test walks the real coverage scope, so a failure here is a gate
  // failure like any other and must report as a FAIL line rather than a stack
  // trace: an undeclared surface is exactly what it is meant to catch.
  let selfTestCounts = null;
  if (selfTest) {
    try {
      selfTestCounts = runSelfTest();
    } catch (err) {
      if (err instanceof GateFailure) fail(err.message);
      throw err;
    }
  }

  // A failure in the coverage walk (a missing scan root, an undeclared surface)
  // is a gate failure like any other and must report as a FAIL line rather than
  // a stack trace, so the walk and the scan share one error boundary.
  let outcomes;
  try {
    if (!selfTest) {
      // The coverage walk is what makes an unmodelled surface fail closed; it runs
      // on every real scan, not only under --self-test.
      const discovered = collectDeclarationSurfaces();
      const expected = [...SCANNED_SURFACES].sort();
      const undeclared = discovered.filter((label) => !SCANNED_SURFACES.includes(label));
      if (undeclared.length > 0) {
        fail(
          `unmodelled surface(s) declare a Lambda runtime but are not judged: ` +
            `${undeclared.join(", ")}; add each to SCANNED_SURFACES in ` +
            `scripts/check-lambda-runtime-deprecations.mjs - surfaces are never skipped`,
        );
      }
      for (const label of expected) {
        if (!discovered.includes(label)) {
          fail(`declared surface ${label} declares no modelled Lambda runtime`);
        }
      }
    }
    outcomes = SCANNED_SURFACES.map((label) => scanSurface(label));
  } catch (err) {
    if (err instanceof GateFailure) fail(err.message);
    throw err;
  }

  const violations = outcomes.flatMap((outcome) => outcome.violations);
  const declarationCount = outcomes.reduce((total, outcome) => total + outcome.declarationCount, 0);

  for (const outcome of outcomes) {
    console.log(`  ${outcome.label} declares ${outcome.declarationCount} Lambda runtime(s)`);
  }

  if (violations.length > 0) {
    for (const violation of violations) {
      console.error(`lambda-runtime-deprecations: ${describeViolation(violation)}`);
    }
    fail(
      `${violations.length} Lambda runtime declaration(s) are deprecated or unmodelled ` +
        `(${declarationCount} declarations across ${outcomes.length} surfaces)`,
    );
  }

  const selfTestSummary = selfTestCounts
    ? `self-test ${selfTestCounts.classifierCases} synthetic surfaces + ${selfTestCounts.scopeCases} scope cases; `
    : "";
  console.log(
    `lambda-runtime-deprecations: PASS (${selfTestSummary}surfaces ${outcomes.length}; ` +
      `declarations ${declarationCount}; deprecated set ${DEPRECATED_LAMBDA_RUNTIMES.length}; ` +
      `supported set ${SUPPORTED_LAMBDA_RUNTIMES.length})`,
  );
}

main();
