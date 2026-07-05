---
title: Consumer Update Automation
description: How consumers find current TableTheory release assets and let Renovate update pinned GitHub Release URLs.
---

# Consumer update automation

TableTheory distributes Go, TypeScript, and Python through immutable GitHub Releases only. There is no npm or PyPI
registry publication. Consumers should therefore pin exact release assets and let automation update those URLs.

## Find the current release

Use GitHub release metadata as the source of truth:

```bash
# Latest stable release tag and URL.
gh release view --repo theory-cloud/TableTheory --json tagName,publishedAt,url

# Recent stable and prerelease tags.
gh release list --repo theory-cloud/TableTheory --exclude-drafts --limit 10
```

Without the GitHub CLI, use the GitHub API:

```bash
curl -fsSL https://api.github.com/repos/theory-cloud/TableTheory/releases/latest | jq -r '.tag_name'
```

Release tags include the leading `v` (`vX.Y.Z`). TypeScript asset names use the same version without the leading `v`:
`theory-cloud-tabletheory-ts-X.Y.Z.tgz`. Python wheel asset names also omit the leading `v`:
`tabletheory_py-X.Y.Z-py3-none-any.whl`. For release candidates, the Git tag keeps the semver prerelease form
(`vX.Y.Z-rc.N`) and the Python wheel uses PEP 440 form (`X.Y.ZrcN`).

## Renovate config for release assets

Renovate's built-in npm and pip managers cannot see TableTheory's direct GitHub Release asset URLs as registry
packages. Use regex custom managers backed by the `github-releases` datasource instead. The config below updates stable
TypeScript tarball and Python wheel URLs in Markdown docs, package manifests, lockfiles, and requirements files.

```json
{
  "$schema": "https://docs.renovatebot.com/renovate-schema.json",
  "extends": ["config:recommended"],
  "customManagers": [
    {
      "customType": "regex",
      "description": "Update TableTheory TypeScript GitHub Release asset URLs",
      "managerFilePatterns": [
        "/(^|/)package\\.json$/",
        "/(^|/)package-lock\\.json$/",
        "/(^|/)README\\.md$/",
        "/(^|/)docs/.+\\.md$/"
      ],
      "matchStrings": [
        "https://github\\.com/theory-cloud/[Tt]able[Tt]heory/releases/download/v(?<currentValue>\\d+\\.\\d+\\.\\d+)/theory-cloud-tabletheory-ts-(?<assetVersion>\\d+\\.\\d+\\.\\d+)\\.tgz"
      ],
      "datasourceTemplate": "github-releases",
      "depNameTemplate": "theory-cloud/TableTheory",
      "versioningTemplate": "semver",
      "extractVersionTemplate": "^v(?<version>\\d+\\.\\d+\\.\\d+)$",
      "autoReplaceStringTemplate": "https://github.com/theory-cloud/TableTheory/releases/download/v{{{newValue}}}/theory-cloud-tabletheory-ts-{{{newValue}}}.tgz"
    },
    {
      "customType": "regex",
      "description": "Update TableTheory Python wheel GitHub Release asset URLs",
      "managerFilePatterns": [
        "/(^|/)requirements.*\\.txt$/",
        "/(^|/)README\\.md$/",
        "/(^|/)docs/.+\\.md$/"
      ],
      "matchStrings": [
        "https://github\\.com/theory-cloud/[Tt]able[Tt]heory/releases/download/v(?<currentValue>\\d+\\.\\d+\\.\\d+)/tabletheory_py-(?<assetVersion>\\d+\\.\\d+\\.\\d+)-py3-none-any\\.whl"
      ],
      "datasourceTemplate": "github-releases",
      "depNameTemplate": "theory-cloud/TableTheory",
      "versioningTemplate": "semver",
      "extractVersionTemplate": "^v(?<version>\\d+\\.\\d+\\.\\d+)$",
      "autoReplaceStringTemplate": "https://github.com/theory-cloud/TableTheory/releases/download/v{{{newValue}}}/tabletheory_py-{{{newValue}}}-py3-none-any.whl"
    }
  ]
}
```

Notes:

- The snippet intentionally tracks stable `X.Y.Z` assets. Release-candidate pins (`X.Y.Z-rc.N` / `X.Y.ZrcN`) are
  validation choices and should be advanced deliberately.
- If your Renovate configuration restricts enabled managers, include `"custom.regex"` in `enabledManagers`.
- Keep `--save-exact` for TypeScript installs and `==X.Y.Z` constraints for Python when you want reproducible builds.
