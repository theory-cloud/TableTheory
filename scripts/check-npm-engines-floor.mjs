// Purpose: fail when a dependency in the repo's npm lockfile set declares an
// engines.node range that excludes the repository's Node floor.
//
// ===========================================================================
// Floor semantics
// ===========================================================================
// ts/package.json declares the repository's Node floor as an exact `>=N` major
// claim, so this checker reads the floor from that manifest rather than
// duplicating it. The floor is additionally pinned to POLICY_FLOOR_MAJOR, so
// the gate cannot be weakened by lowering the manifest: the two must agree.
//
// The matching CI legs install the newest Node N.x release, which makes the
// floor runtime the whole `N.x` line: a dependency passes only when its
// declared engines.node range admits at least one Node N.x release.
//
//   floor >=22 | engines >=22                  -> pass
//   floor >=22 | engines ^22.13.0              -> pass (22.13.x is a 22.x)
//   floor >=22 | engines 18 || 20 || >=22      -> pass
//   floor >=22 | engines >=24                  -> FAIL
//   floor >=22 | engines ^20.19.0 || >=24      -> FAIL
//   floor >=22 | engines >=23.0.0-0            -> FAIL (23.0.0-0 sorts above
//                                                 every 22.x release, so no
//                                                 floor release satisfies it)
//   floor >=22 | engines =22.13.0-rc.1         -> FAIL (matches a single
//                                                 prerelease, and a prerelease
//                                                 is not a release)
//   floor >=22 | engines >22.0.0 <22.0.1       -> FAIL (no release lies strictly
//                                                 between adjacent bounds)
//   floor >=20 | engines >=22                  -> FAIL (the drift class this
//                                                 gate exists to catch: a
//                                                 locked dependency that cannot
//                                                 run on the floor it ships to)
//
// A range admits the floor only when some concrete release (a version with no
// prerelease component) satisfies it, and prereleases of the line's upper edge
// sort above every release on the line: `>=23.0.0-0` admits no 22.x release
// even though 23.0.0-0 itself lies above 22.0.0. Bounds are therefore
// normalized to the release edge they delimit before the bands are compared,
// which keeps the predicate exact in release space.
//
// The matcher below is deliberately self-contained: a dependency gate must not
// depend on a transitive npm package, and it must model every range it can
// meet rather than skip the ones it cannot. Any range grammar it does not
// model fails the gate outright - the legacy tilde alias `~>` included.
//
// ===========================================================================
// Root semantics (one rule, converged across the framework repos)
// ===========================================================================
// Every audited lockfile's own root entry ("") must DECLARE engines.node, and
// the declaration must not admit any release below the repository floor major.
// The two halves fail differently and both fail closed:
//
//   absent declaration        -> FAIL (a project that never states the Node
//                                line it ships to is the hole this rule closes)
//   admits below the floor    -> FAIL
//   >=22 on a floor of 22     -> pass
//   >=24 on a floor of 22     -> pass (declaring a HIGHER floor is legitimate;
//                                the rule is about the lower edge, not about
//                                matching the repository floor exactly)
//   ^20.19.0 || >=22          -> FAIL (the first branch still admits 20.x)
//   * / >=20 / 22.x           -> * fails, >=20 fails, 22.x passes
//
// An example project that ships only on Node 24 may therefore keep declaring
// >=24, while a project that declares nothing - or that still admits Node 20 -
// cannot pass. This is the same shape FaceTheory already enforces and that
// AppTheory is converging on in a parallel wave.
//
// A root is deliberately NOT held to the dependency rule, which is stricter in
// the other direction: a dependency must admit at least one floor release
// (>=24 fails), while a root must not admit a sub-floor release (>=24 passes).
// The two directions are different questions and are evaluated by different
// predicates below.
//
// ===========================================================================
// Scope
// ===========================================================================
// Dependency entries (node_modules/**) in the lockfiles that
// scripts/sec-npm-audit.sh audits: ts/package-lock.json,
// contract-tests/runners/ts/package-lock.json, and
// examples/cdk-multilang/package-lock.json. Positional arguments replace that
// set, which is how the negative proof drives the checker with a synthetic
// lockfile.
//
// AUDITED_LOCKFILES is not allowed to drift from the audited set by hand. Every
// real scan asserts that the set matches the prefixes scripts/sec-npm-audit.sh
// audits, so a lockfile cannot be audited by one gate and skipped by the other.
//
// There is no exception list and no waiver flag: every dependency entry with a
// declared engines.node must admit the floor, and every root must declare a
// floor that does not admit a release below it, or the gate fails.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
// The repository's Node floor lives in ts/package.json. Node 20 reached end of
// life in April 2026, which is why the policy floor is 22.
const FLOOR_MANIFEST = "ts/package.json";
const POLICY_FLOOR_MAJOR = 22;
const AUDITED_LOCKFILES = [
  "ts/package-lock.json",
  "contract-tests/runners/ts/package-lock.json",
  "examples/cdk-multilang/package-lock.json",
];

function fail(message) {
  console.error(`npm-engines-floor: FAIL (${message})`);
  process.exit(1);
}

function readJson(relativePath, description) {
  return readJsonFile(path.join(repositoryRoot, relativePath), description, relativePath);
}

function readJsonFile(absolutePath, description, label) {
  let text;
  try {
    text = fs.readFileSync(absolutePath, "utf8");
  } catch (err) {
    fail(`could not read ${description} ${label}: ${err.message}`);
  }
  try {
    return JSON.parse(text);
  } catch (err) {
    fail(`could not parse ${description} ${label}: ${err.message}`);
  }
}

// === floor =================================================================

function readFloorMajor() {
  const manifest = readJson(FLOOR_MANIFEST, "floor manifest");
  const spec = manifest?.engines?.node;
  if (typeof spec !== "string") {
    fail(`${FLOOR_MANIFEST} must declare engines.node as a string`);
  }
  const match = /^\s*>=\s*(\d+)(?:\.\d+){0,2}\s*$/.exec(spec);
  if (match === null) {
    fail(
      `${FLOOR_MANIFEST} engines.node must use an exact >=N Node major floor, found ${JSON.stringify(spec)}`,
    );
  }
  const major = Number(match[1]);
  if (major !== POLICY_FLOOR_MAJOR) {
    fail(
      `${FLOOR_MANIFEST} engines.node is ${JSON.stringify(spec)} but this gate's policy floor is ` +
        `Node ${POLICY_FLOOR_MAJOR}; move the policy floor and the manifest together, never weaken one alone`,
    );
  }
  return { major, spec };
}

// === semver range support ==================================================

class UnmodelledRange extends Error {}

const PARTIAL_RE = /^v?(\d+|[xX*])(?:\.(\d+|[xX*]))?(?:\.(\d+|[xX*]))?(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$/;

function parsePartial(raw) {
  const match = PARTIAL_RE.exec(raw.trim());
  if (match === null) return null;
  const component = (value) => {
    if (value === undefined || /^[xX*]$/.test(value)) return null;
    return Number(value);
  };
  const wildcardComponent = (value) => value !== undefined && /^[xX*]$/.test(value);
  const major = component(match[1]);
  const minor = component(match[2]);
  const patch = component(match[3]);
  const prerelease = match[4] ?? null;
  // A concrete component must not follow a wildcard component. The regex
  // above accepts `22.x.1` syntactically, but npm's semver rejects it, so
  // accepting it would mean silently coercing an unmodelled range to `22.x`
  // and reporting a floor result the declaration does not actually make.
  if (wildcardComponent(match[2]) && match[3] !== undefined && !wildcardComponent(match[3])) return null;
  if (major === null) {
    // `*`, `x`, `X` and `v*` leave the major unconstrained.
    if (minor !== null || patch !== null || prerelease !== null) return null;
    return { any: true };
  }
  const wildcarded = minor === null || patch === null;
  if (wildcarded && prerelease !== null) return null;
  return {
    any: false,
    major,
    minor: minor ?? 0,
    patch: patch === null ? 0 : patch,
    prerelease,
    minorWildcard: minor === null,
    patchWildcard: minor !== null && patch === null,
  };
}

// A concrete version for comparisons: partial components already default to 0.
function concrete(version) {
  return {
    major: version.major,
    minor: version.minor,
    patch: version.patch,
    prerelease: version.prerelease ?? null,
  };
}

function bump(version, level) {
  if (level === "major") return { major: version.major + 1, minor: 0, patch: 0, prerelease: null };
  if (level === "minor") return { major: version.major, minor: version.minor + 1, patch: 0, prerelease: null };
  return { major: version.major, minor: version.minor, patch: version.patch + 1, prerelease: null };
}

function comparePrerelease(a, b) {
  const left = a.split(".");
  const right = b.split(".");
  for (let index = 0; index < Math.max(left.length, right.length); index += 1) {
    const l = left[index];
    const r = right[index];
    if (l === undefined) return -1;
    if (r === undefined) return 1;
    const lNumeric = /^\d+$/.test(l);
    const rNumeric = /^\d+$/.test(r);
    if (lNumeric && rNumeric) {
      if (Number(l) !== Number(r)) return Number(l) < Number(r) ? -1 : 1;
      continue;
    }
    if (lNumeric !== rNumeric) return lNumeric ? -1 : 1;
    if (l !== r) return l < r ? -1 : 1;
  }
  return 0;
}

function compareVersions(a, b) {
  for (const key of ["major", "minor", "patch"]) {
    if (a[key] !== b[key]) return a[key] < b[key] ? -1 : 1;
  }
  if (a.prerelease === b.prerelease) return 0;
  if (a.prerelease === null) return 1;
  if (b.prerelease === null) return -1;
  return comparePrerelease(a.prerelease, b.prerelease);
}

function maxLower(a, b) {
  if (a === null) return b;
  if (b === null) return a;
  const order = compareVersions(a.version, b.version);
  if (order > 0) return a;
  if (order < 0) return b;
  return { version: a.version, inclusive: a.inclusive && b.inclusive };
}

function minUpper(a, b) {
  if (a === null) return b;
  if (b === null) return a;
  const order = compareVersions(a.version, b.version);
  if (order < 0) return a;
  if (order > 0) return b;
  return { version: a.version, inclusive: a.inclusive && b.inclusive };
}

function applyXRange(version, bounds) {
  if (version.minorWildcard) {
    bounds.lower = maxLower(bounds.lower, { version: concrete(version), inclusive: true });
    bounds.upper = minUpper(bounds.upper, { version: bump(version, "major"), inclusive: false });
    return;
  }
  if (version.patchWildcard) {
    bounds.lower = maxLower(bounds.lower, { version: concrete(version), inclusive: true });
    bounds.upper = minUpper(bounds.upper, { version: bump(version, "minor"), inclusive: false });
    return;
  }
  bounds.lower = maxLower(bounds.lower, { version: concrete(version), inclusive: true });
  bounds.upper = minUpper(bounds.upper, { version: concrete(version), inclusive: true });
}

function caretUpper(version) {
  if (version.major > 0 || version.minorWildcard) return bump(version, "major");
  if (version.minor > 0 || version.patchWildcard) return bump(version, "minor");
  return bump(version, "patch");
}

function applyComparator(rawToken, bounds) {
  const token = rawToken.trim();
  if (token === "" || token === "*" || /^[xX]$/.test(token)) return;

  const match = /^(>=|<=|>|<|=|\^|~)?(.+)$/.exec(token);
  if (match === null) throw new UnmodelledRange(token);
  const operator = match[1] ?? "";
  const version = parsePartial(match[2]);
  if (version === null) throw new UnmodelledRange(token);
  if (version.any) {
    if (operator === "" || operator === "=") return;
    throw new UnmodelledRange(token);
  }

  const lower = (candidate, inclusive) => {
    bounds.lower = maxLower(bounds.lower, { version: candidate, inclusive });
  };
  const upper = (candidate, inclusive) => {
    bounds.upper = minUpper(bounds.upper, { version: candidate, inclusive });
  };

  switch (operator) {
    case ">=":
      lower(concrete(version), true);
      return;
    case ">":
      if (version.minorWildcard) lower(bump(version, "major"), true);
      else if (version.patchWildcard) lower(bump(version, "minor"), true);
      else lower(concrete(version), false);
      return;
    case "<=":
      if (version.minorWildcard) upper(bump(version, "major"), false);
      else if (version.patchWildcard) upper(bump(version, "minor"), false);
      else upper(concrete(version), true);
      return;
    case "<":
      upper(concrete(version), false);
      return;
    case "=":
    case "":
      applyXRange(version, bounds);
      return;
    case "^":
      lower(concrete(version), true);
      upper(caretUpper(version), false);
      return;
    case "~":
      lower(concrete(version), true);
      upper(version.minorWildcard ? bump(version, "major") : bump(version, "minor"), false);
      return;
    default:
      throw new UnmodelledRange(token);
  }
}

function applyHyphen(leftText, rightText, bounds) {
  const lowerVersion = parsePartial(leftText);
  const upperVersion = parsePartial(rightText);
  if (lowerVersion === null || upperVersion === null || lowerVersion.any || upperVersion.any) {
    throw new UnmodelledRange(`${leftText} - ${rightText}`);
  }
  bounds.lower = maxLower(bounds.lower, { version: concrete(lowerVersion), inclusive: true });
  if (upperVersion.minorWildcard) {
    bounds.upper = minUpper(bounds.upper, { version: bump(upperVersion, "major"), inclusive: false });
  } else if (upperVersion.patchWildcard) {
    bounds.upper = minUpper(bounds.upper, { version: bump(upperVersion, "minor"), inclusive: false });
  } else {
    bounds.upper = minUpper(bounds.upper, { version: concrete(upperVersion), inclusive: true });
  }
}

function branchBounds(rawBranch) {
  // `>= 4.0.0` and `>=4.0.0` are the same range; glue the operator back onto its
  // version so whitespace splitting only separates comparator sets.
  const text = rawBranch.replace(/(>=|<=|>|<|=|\^|~)\s+/g, "$1").trim();
  const bounds = { lower: null, upper: null };
  if (text === "" || text === "*" || /^[xX]$/.test(text)) return bounds;

  const hyphen = /^([^\s]+)\s+-\s+([^\s]+)$/.exec(text);
  if (hyphen !== null) {
    applyHyphen(hyphen[1], hyphen[2], bounds);
    return bounds;
  }

  for (const token of text.split(/\s+/)) applyComparator(token, bounds);
  return bounds;
}

// === release-space intersection ============================================
// A band is a pair of version bounds; `null` means unbounded on that side. The
// bounds are compared over concrete releases, so a prerelease bound collapses
// to the release edge it delimits:
//   lower `23.0.0-x` inclusive -> `23.0.0` inclusive (23.0.0 is the first release >= it)
//   lower `23.0.0-x` exclusive -> `23.0.0` inclusive (23.0.0 is the first release > it)
//   upper `23.0.0-x` inclusive -> `23.0.0` exclusive (no release <= it reaches 23.0.0)
//   upper `23.0.0-x` exclusive -> `23.0.0` exclusive (no release lies between them)

function toReleaseLower(bound) {
  if (bound === null || bound.version.prerelease === null) return bound;
  return { version: { ...bound.version, prerelease: null }, inclusive: true };
}

function toReleaseUpper(bound) {
  if (bound === null || bound.version.prerelease === null) return bound;
  return { version: { ...bound.version, prerelease: null }, inclusive: false };
}

const ZERO_RELEASE = { major: 0, minor: 0, patch: 0, prerelease: null };

// Smallest concrete release admitted by an already-normalized lower bound.
function firstRelease(lower) {
  if (lower.inclusive) return lower.version;
  return bump(lower.version, "patch");
}

// True when some concrete release satisfies both the range bounds and the band.
function bandAdmitsRelease(bounds, band) {
  const lower = maxLower(toReleaseLower(bounds.lower), toReleaseLower(band.lower));
  const upper = minUpper(toReleaseUpper(bounds.upper), toReleaseUpper(band.upper));
  if (upper === null) return true;
  if (lower === null) {
    // The band's upper edge is itself a release, so it witnesses the band
    // unless it is exclusive at the 0.0.0 floor.
    return upper.inclusive || compareVersions(upper.version, ZERO_RELEASE) > 0;
  }
  const order = compareVersions(firstRelease(lower), upper.version);
  return upper.inclusive ? order <= 0 : order < 0;
}

// Bands are half-open release intervals: [major.0.0, (major + 1).0.0).
function bandForMajor(major) {
  return {
    lower: { version: { major, minor: 0, patch: 0, prerelease: null }, inclusive: true },
    upper: { version: { major: major + 1, minor: 0, patch: 0, prerelease: null }, inclusive: false },
  };
}

// Every branch is parsed before any of them is evaluated, so an unmodelled
// branch fails the gate even when an earlier branch already admits the band;
// strictness must not depend on the order of the alternatives.
function rangeAdmitsBand(rawRange, band) {
  const range = String(rawRange);
  if (range.trim() === "") return true;
  return range
    .split("||")
    .map((branch) => branchBounds(branch))
    .some((bounds) => bandAdmitsRelease(bounds, band));
}

// True when the declared range admits at least one release on the floor's
// major line, i.e. when a release in [floorMajor.0.0, (floorMajor + 1).0.0)
// satisfies it. This is the DEPENDENCY rule.
function rangeAdmitsFloorMajor(rawRange, floorMajor) {
  return rangeAdmitsBand(rawRange, bandForMajor(floorMajor));
}

// True when the declared range admits at least one concrete release BELOW the
// floor major, i.e. when a release in [0.0.0, floorMajor.0.0) satisfies it.
// This is the inverse question the ROOT rule asks: a project root passes when
// this is false, so `>=22` and `>=24` both pass on a floor of 22 while `>=20`,
// `*`, and `^20.19.0 || >=22` all fail.
function rangeAdmitsBelowFloorMajor(rawRange, floorMajor) {
  return rangeAdmitsBand(rawRange, {
    lower: { version: { major: 0, minor: 0, patch: 0, prerelease: null }, inclusive: true },
    upper: { version: { major: floorMajor, minor: 0, patch: 0, prerelease: null }, inclusive: false },
  });
}

// === lockfile set ==========================================================

function defaultLockfiles() {
  const lockfiles = AUDITED_LOCKFILES.map((relative) => ({
    absolute: path.join(repositoryRoot, relative),
    label: relative,
  }));
  // Fail closed if the audit-covered set goes missing: a shrunken lockfile set
  // must never look like a pass.
  for (const lockfile of lockfiles) {
    if (!fs.existsSync(lockfile.absolute)) {
      fail(`missing audited lockfile ${lockfile.label}; the npm lockfile set must stay in sync with scripts/sec-npm-audit.sh`);
    }
  }
  return lockfiles;
}

// Positional arguments replace the audit-scanner lockfile set so the negative
// proof can point the checker at a synthetic lockfile. Every lockfile is still
// judged by the same rules; there is no way to exempt one.
function resolveLockfileArgument(value) {
  const absolute = path.isAbsolute(value) ? value : path.join(repositoryRoot, value);
  const relative = path.relative(repositoryRoot, absolute).split(path.sep).join("/");
  // Out-of-tree lockfiles (the synthetic negative-proof case) read better as
  // absolute paths than as a wall of "../".
  const label = relative === "" || relative.startsWith("../") ? absolute : relative;
  return { absolute, label };
}

// === scan ==================================================================

// Runs a range predicate, converting the matcher's unmodelled-grammar signal
// into a gate failure that names the declaration. Ranges are never skipped: a
// grammar the matcher cannot read fails the gate, wherever it appears.
function judgeRange(rawRange, floorMajor, predicate, where) {
  try {
    return predicate(rawRange, floorMajor);
  } catch (err) {
    if (err instanceof UnmodelledRange) {
      fail(
        `${where} declares engines.node ${JSON.stringify(rawRange)}, ` +
          `which scripts/check-npm-engines-floor.mjs does not model (${err.message}); ` +
          "extend the matcher - ranges are never skipped",
      );
    }
    throw err;
  }
}

function scanLockfile(lockfile, floorMajor) {
  const { absolute, label } = lockfile;
  const packages = readJsonFile(absolute, "lockfile", label)?.packages;
  if (packages === undefined || typeof packages !== "object" || packages === null) {
    fail(`${label} is missing its packages map`);
  }

  const result = { checked: 0, judgedRoots: 0, rootViolations: [], violations: [] };

  // Every audited lockfile root must DECLARE engines.node, and the declaration
  // must not admit a release below the repository floor. Absence fails closed:
  // a project that never states the Node line it ships to is the hole this rule
  // exists to close, and a missing declaration must never read as a pass.
  const rootEntry = packages[""];
  if (rootEntry === undefined || typeof rootEntry !== "object" || rootEntry === null) {
    fail(`${label} is missing its root entry (""); an audited lockfile must declare its Node floor`);
  }
  const rootDeclared = rootEntry.engines?.node;
  if (rootDeclared === undefined || rootDeclared === null) {
    fail(
      `${label} <root> declares no engines.node; every audited lockfile root must declare the Node ` +
        `floor it ships to (add engines.node ">=${floorMajor}" to the package manifest and regenerate ` +
        "the lockfile)",
    );
  }
  if (typeof rootDeclared !== "string") {
    fail(`${label} <root> engines.node is not a string`);
  }
  result.judgedRoots += 1;
  if (judgeRange(rootDeclared, floorMajor, rangeAdmitsBelowFloorMajor, `${label} <root>`)) {
    result.rootViolations.push({
      lockfile: label,
      version: rootEntry.version ?? "<unknown>",
      declared: rootDeclared,
    });
  }

  for (const [packagePath, entry] of Object.entries(packages)) {
    if (packagePath === "") continue;
    const declared = entry?.engines?.node;
    if (declared === undefined || declared === null) continue;
    if (typeof declared !== "string") {
      fail(`${label} ${packagePath} engines.node is not a string`);
    }

    const admits = judgeRange(
      declared,
      floorMajor,
      rangeAdmitsFloorMajor,
      `${label} ${packagePath}`,
    );

    result.checked += 1;
    if (!admits) {
      result.violations.push({
        lockfile: label,
        packagePath,
        version: entry?.version ?? "<unknown>",
        declared,
      });
    }
  }
  return result;
}

// Range grammar the matcher deliberately does not model. Each of these must
// throw UnmodelledRange, so an unmodelled range is never silently read as a
// pass. `~>` is the legacy tilde alias; npm's semver accepts it as `~`, but the
// matcher does not model it and fails closed rather than guess. The trailing
// `x.N` forms are semver-invalid, and the matcher must reject them instead of
// coercing them to `22.x`.
const UNMODELLED_CASES = [
  "~>22",
  "~> 22.0.0",
  "~>22.0.0",
  "lts/*",
  ">=22.0.0 || lts/*",
  "22.x.1",
  "22.*.1",
  "22.x.0",
];

function runSelfTest() {
  const FLOOR = 22;
  const cases = [
    // Ranges that admit the floor major.
    ["*", FLOOR, true],
    ["x", FLOOR, true],
    [">=22", FLOOR, true],
    [">=22.0.0", FLOOR, true],
    [">= 22.0.0", FLOOR, true],
    [">=20", FLOOR, true],
    [">= 0.4", FLOOR, true],
    [">= 4", FLOOR, true],
    [">=16.20.0", FLOOR, true],
    [">= 20.16.0", FLOOR, true],
    [">=21", FLOOR, true],
    [">21", FLOOR, true],
    ["<=22", FLOOR, true],
    ["<=22.0.0", FLOOR, true],
    ["<23", FLOOR, true],
    ["<23.0.0", FLOOR, true],
    ["22.x", FLOOR, true],
    ["22.4.x", FLOOR, true],
    ["=22.0.0", FLOOR, true],
    ["22.0.0", FLOOR, true],
    ["22", FLOOR, true],
    ["22.0", FLOOR, true],
    ["^22.13.0", FLOOR, true],
    ["^20.19.0 || ^22.13.0 || >=24", FLOOR, true],
    ["^18.18.0 || ^20.9.0 || >=21.1.0", FLOOR, true],
    ["^20.10.0 || >=21.0.0", FLOOR, true],
    ["18 || 20 || >=22", FLOOR, true],
    ["20 || >=22", FLOOR, true],
    ["^14.17.0 || ^16.0.0 || >=18.0.0", FLOOR, true],
    ["^12.22.0 || ^14.17.0 || >=16.0.0", FLOOR, true],
    ["6.* || 8.* || >= 10.*", FLOOR, true],
    [">=6 <7 || >=8", FLOOR, true],
    ["^6 || ^7 || ^8 || ^9 || ^10 || ^11 || ^12 || >=13.7", FLOOR, true],
    ["~22.1.0", FLOOR, true],
    ["21 - 22", FLOOR, true],
    ["20.1 - 22.3.4", FLOOR, true],
    ["22.0.0 - 22.0.5", FLOOR, true],
    ["1 - 22", FLOOR, true],
    [">=22.13.0-0", FLOOR, true],
    ["<24.0.0", FLOOR, true],
    ["^20.19.0 || ^22.13.0 || >=24.0.0", FLOOR, true],
    // Prerelease bounds on the floor line itself still admit floor releases.
    [">=22.0.0-0", FLOOR, true],
    ["^22.0.0-0", FLOOR, true],
    ["<=23.0.0-0", FLOOR, true],
    [">=22.0.0-0 <23.0.0", FLOOR, true],
    ["<23.0.0-0", FLOOR, true],
    ["22.0.0-0 - 22.9.9", FLOOR, true],
    // Ranges that exclude the floor major.
    [">=24", FLOOR, false],
    [">24", FLOOR, false],
    ["^24.0.0", FLOOR, false],
    ["24.x", FLOOR, false],
    ["^20.19.0 || >=24", FLOOR, false],
    ["^20.19.0 || ^23.0.0 || >=24", FLOOR, false],
    [">=26", FLOOR, false],
    ["<22", FLOOR, false],
    ["<=21", FLOOR, false],
    ["<=21.9", FLOOR, false],
    ["<22.0.0", FLOOR, false],
    ["18 || 20", FLOOR, false],
    [">=6 <7", FLOOR, false],
    ["^6 || ^7 || ^8", FLOOR, false],
    ["0.10.x", FLOOR, false],
    ["^0.2.3", FLOOR, false],
    ["23.x", FLOOR, false],
    ["21 - 21.9", FLOOR, false],
    ["18.0.0 - 20.19.5", FLOOR, false],
    // Prereleases of the next line's release sort above every floor release, so
    // they admit no floor release at all and must fail.
    [">=23.0.0-0", FLOOR, false],
    ["23.0.0-0", FLOOR, false],
    ["v23.0.0-0", FLOOR, false],
    ["=23.0.0-rc.0", FLOOR, false],
    ["^23.0.0-alpha", FLOOR, false],
    ["~23.0.0-beta", FLOOR, false],
    [">23.0.0-0", FLOOR, false],
    [">23.0.0-beta.1", FLOOR, false],
    [">=23.0.0-0 <23.0.0", FLOOR, false],
    // A prerelease anchored range that matches only prereleases is not a
    // release range, even when the prerelease sits on the floor's own line.
    ["=22.13.0-rc.1", FLOOR, false],
    ["23.0.0-0 - 24.0.0", FLOOR, false],
    // Disjunctions mixing a release branch with a prerelease-anchored branch
    // are judged branch by branch, so no branch may smuggle the floor in.
    ["^20.19.0 || >=23.0.0-0", FLOOR, false],
    ["^18.18.0 || ^20.9.0 || >=23.0.0-0", FLOOR, false],
    // Exclusive bounds on adjacent releases admit no release either.
    [">22.0.0 <22.0.1", FLOOR, false],
    // The stream-chain class against the previous floor: this is the drift the
    // gate exists to catch.
    [">=22", 20, false],
    [">=20", 20, true],
    [">= 20.16.0", 20, true],
    [">=24", 22, false],
  ];

  // [range, floor, admits a release below the floor]. The root rule asks the
  // INVERSE of the dependency rule: a root passes when it does not admit a
  // sub-floor release, so `>=24` passes on a floor of 22 while `>=20` fails.
  const rootCases = [
    // Declarations at or above the floor: no sub-floor release is admitted.
    [">=22", FLOOR, false],
    [">=24", FLOOR, false],
    [">=22.0.0", FLOOR, false],
    [">= 22.0.0", FLOOR, false],
    ["22", FLOOR, false],
    ["22.0", FLOOR, false],
    ["=22.0.0", FLOOR, false],
    ["22.x", FLOOR, false],
    ["22.4.x", FLOOR, false],
    ["^22.13.0", FLOOR, false],
    ["~22.1.0", FLOOR, false],
    ["23.x", FLOOR, false],
    ["^24.0.0", FLOOR, false],
    [">=24 <25", FLOOR, false],
    [">=26", FLOOR, false],
    [">=22 <23", FLOOR, false],
    [">=22.0.0-0", FLOOR, false],
    [">=22.13.0-0", FLOOR, false],
    ["^22.0.0-0", FLOOR, false],
    [">=23.0.0-0", FLOOR, false],
    [">=22.0.0-0 <23.0.0", FLOOR, false],
    ["22.0.0-0 - 22.9.9", FLOOR, false],
    // `>21` desugars to `>=22.0.0` in node-semver - the whole 21.x line is
    // excluded - so it admits no sub-floor release even though the bound it
    // names sits below the floor.
    [">21", FLOOR, false],
    ["22.0.0 - 22.0.5", FLOOR, false],
    // The FaceTheory "root false-fail" class: a prerelease anchored on the
    // floor's own line sorts above every sub-floor release, so it must not be
    // reported as admitting one.
    [">=22.0.0-0 <23", FLOOR, false],
    // A stricter root than the repository floor is legitimate.
    [">=24", 20, false],
    [">=22", 20, false],
    [">=20", 20, false],
    // Declarations that still admit a sub-floor release.
    [">=20", FLOOR, true],
    [">=18", FLOOR, true],
    [">=18", 20, true],
    ["*", FLOOR, true],
    ["x", FLOOR, true],
    [">=0.4", FLOOR, true],
    ["20", FLOOR, true],
    ["20.x", FLOOR, true],
    ["^20.19.0", FLOOR, true],
    ["^20.19.0 || >=22", FLOOR, true],
    ["^20.19.0 || ^22.13.0 || >=24", FLOOR, true],
    ["^18.18.0 || ^20.9.0 || >=21.1.0", FLOOR, true],
    ["18 || 20", FLOOR, true],
    ["20 || >=22", FLOOR, true],
    ["<=22", FLOOR, true],
    ["<22", FLOOR, true],
    ["<=21", FLOOR, true],
    ["<23.0.0", FLOOR, true],
    ["<=23.0.0-0", FLOOR, true],
    [">=21", FLOOR, true],
    ["21 - 21.9", FLOOR, true],
    ["21 - 22", FLOOR, true],
    ["1 - 22", FLOOR, true],
    ["20.1 - 22.3.4", FLOOR, true],
    ["^14.17.0 || ^16.0.0 || >=18.0.0", FLOOR, true],
    ["0.10.x", FLOOR, true],
  ];

  let failures = 0;
  for (const [range, floor, expected] of cases) {
    let actual;
    try {
      actual = rangeAdmitsFloorMajor(range, floor);
    } catch (err) {
      console.error(`  self-test: ${JSON.stringify(range)} at floor ${floor} threw ${err.message}`);
      failures += 1;
      continue;
    }
    if (actual !== expected) {
      console.error(
        `  self-test: ${JSON.stringify(range)} at floor ${floor} expected ${expected}, got ${actual}`,
      );
      failures += 1;
    }
  }

  // The root rule is judged by its own predicate, so it is self-tested against
  // its own table: an inverted expectation here would silently pass every root.
  for (const [range, floor, expectedAdmitsBelow] of rootCases) {
    let actual;
    try {
      actual = rangeAdmitsBelowFloorMajor(range, floor);
    } catch (err) {
      console.error(`  self-test root: ${JSON.stringify(range)} at floor ${floor} threw ${err.message}`);
      failures += 1;
      continue;
    }
    if (actual !== expectedAdmitsBelow) {
      console.error(
        `  self-test root: ${JSON.stringify(range)} at floor ${floor} ` +
          `expected admits-below-floor ${expectedAdmitsBelow}, got ${actual}`,
      );
      failures += 1;
    }
  }

  // Grammar the matcher does not model must fail closed rather than be skipped,
  // and it must do so whichever branch or position it appears in - for the
  // dependency predicate and the root predicate alike.
  for (const range of UNMODELLED_CASES) {
    for (const [label, predicate] of [
      ["dependency", rangeAdmitsFloorMajor],
      ["root", rangeAdmitsBelowFloorMajor],
    ]) {
      let thrown = null;
      try {
        predicate(range, FLOOR);
      } catch (err) {
        thrown = err;
      }
      if (!(thrown instanceof UnmodelledRange)) {
        console.error(
          `  self-test ${label}: ${JSON.stringify(range)} must fail closed as unmodelled, ` +
            `but ${thrown === null ? "was accepted" : `threw ${thrown.message}`}`,
        );
        failures += 1;
      }
    }
  }

  const total = cases.length + rootCases.length + 2 * UNMODELLED_CASES.length;
  if (failures > 0) fail(`self-test failed (${failures} of ${total} cases)`);
  return total;
}

function main() {
  const args = process.argv.slice(2);
  const selfTest = args[0] === "--self-test";
  const lockfileArgs = selfTest ? args.slice(1) : args;

  const selfTestCases = selfTest ? runSelfTest() : 0;

  const { major: floorMajor, spec: floorSpec } = readFloorMajor();
  const lockfiles =
    lockfileArgs.length > 0 ? lockfileArgs.map(resolveLockfileArgument) : defaultLockfiles();

  let checked = 0;
  let judgedRoots = 0;
  const rootViolations = [];
  const violations = [];
  for (const lockfile of lockfiles) {
    const result = scanLockfile(lockfile, floorMajor);
    checked += result.checked;
    judgedRoots += result.judgedRoots;
    rootViolations.push(...result.rootViolations);
    violations.push(...result.violations);
  }

  for (const violation of rootViolations) {
    console.error(
      `npm-engines-floor: lockfile root declares engines.node ${JSON.stringify(violation.declared)} ` +
        `in ${violation.lockfile} <root>@${violation.version}, which admits a release below the ` +
        `Node ${floorMajor} floor (${FLOOR_MANIFEST} declares ${JSON.stringify(floorSpec)})`,
    );
  }
  for (const violation of violations) {
    console.error(
      `npm-engines-floor: unexpected engines.node ${JSON.stringify(violation.declared)} in ` +
        `${violation.packagePath}@${violation.version} from ${violation.lockfile} ` +
        `(excludes Node ${floorMajor}.x, the ${FLOOR_MANIFEST} floor ${JSON.stringify(floorSpec)})`,
    );
  }
  if (rootViolations.length > 0 || violations.length > 0) {
    const causes = [];
    if (rootViolations.length > 0) {
      causes.push(
        `${rootViolations.length} of ${judgedRoots} lockfile root(s) admit a release below the Node ${floorMajor} floor`,
      );
    }
    if (violations.length > 0) {
      causes.push(
        `${violations.length} of ${checked} npm dependency engine ranges exclude the Node ${floorMajor} floor`,
      );
    }
    fail(causes.join("; "));
  }

  const selfTestSummary = selfTest ? `self-test ${selfTestCases} cases; ` : "";
  console.log(
    `npm-engines-floor: PASS (${selfTestSummary}floor ${JSON.stringify(floorSpec)}; ` +
      `lockfiles ${lockfiles.length}; project roots judged ${judgedRoots}; ` +
      `dependency engine ranges ${checked}; excluded 0)`,
  );
}

main();
