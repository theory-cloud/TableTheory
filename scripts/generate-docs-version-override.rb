#!/usr/bin/env ruby
# frozen_string_literal: true

# =============================================================
# TableTheory · docs version override generator
#
# Writes docs/_config_versions.yml — a Jekyll config override that stamps the
# docs site with a real release tag. .github/workflows/pages.yml resolves the
# deploy tag (the workflow_dispatch `tag` input handed off by release.yml
# after a stable publish, or the latest published non-prerelease tag for
# pushes and bare manual dispatch), calls this script, and builds with
# `--config _config.yml,_config_versions.yml`, so the site's version pill and
# runtime install commands always match the deployed release instead of the
# hand-maintained pins in docs/_config.yml (`version_pill: "v1.x"`,
# `vX.Y.Z` placeholder install URLs).
#
# Jekyll config merging (`jekyll build --config a,b`) deep-merges hashes but
# REPLACES arrays wholesale — the override therefore carries the COMPLETE
# tabletheory.runtimes array from docs/_config.yml, with every `vX.Y.Z` /
# `X.Y.Z` occurrence in each install string replaced by the tag (asset
# filenames strip the leading `v`). docs/_config.yml itself is never modified
# and remains the local-preview fallback; docs/_config_versions.yml is a
# CI/local-gate generated artifact and is git-ignored.
#
# The override is generated only from the release tag — no secrets, no
# branch-state input.
# =============================================================

require "optparse"
require "yaml"

DEFAULT_CONFIG = "docs/_config.yml"
DEFAULT_OUTPUT = "docs/_config_versions.yml"

# Replace every `vX.Y.Z` / `X.Y.Z` placeholder occurrence in an install string
# with the release tag. Asset filenames use the bare version (no leading `v`),
# so the longer `vX.Y.Z` token is stamped first and the bare `X.Y.Z` second.
def stamp_install(install, tag, bare)
  install.gsub("vX.Y.Z", tag).gsub("X.Y.Z", bare)
end

def main(argv)
  options = {}
  OptionParser.new do |opts|
    opts.banner = "Usage: #{$PROGRAM_NAME} --tag TAG [--config PATH] [--output PATH]"
    opts.on("--config PATH", "Source Jekyll config (default: #{DEFAULT_CONFIG})") do |path|
      options[:config] = path
    end
    opts.on("--tag TAG", "Release tag to stamp (e.g. v3.0.6)") do |tag|
      options[:tag] = tag
    end
    opts.on("--output PATH", "Output override path (default: #{DEFAULT_OUTPUT})") do |path|
      options[:output] = path
    end
  end.parse!(argv)

  config_path = options[:config] || DEFAULT_CONFIG
  tag = options[:tag]
  output_path = options[:output] || DEFAULT_OUTPUT

  abort "error: --tag is required (e.g. v3.0.6)" if tag.nil? || tag.empty?
  abort "error: config not found: #{config_path}" unless File.file?(config_path)

  config = YAML.safe_load(File.read(config_path))
  runtimes = config.dig("tabletheory", "runtimes")
  abort "error: #{config_path} has no tabletheory.runtimes array" unless runtimes.is_a?(Array)

  bare = tag.start_with?("v") ? tag[1..] : tag

  stamped = runtimes.map do |runtime|
    runtime = runtime.dup
    install = runtime["install"]
    runtime["install"] = stamp_install(install, tag, bare) if install.is_a?(String)
    runtime
  end

  override = { "tabletheory" => { "version_pill" => tag, "runtimes" => stamped } }

  File.write(output_path, YAML.dump(override, line_width: -1))
  puts "wrote #{output_path} (version_pill=#{tag}, #{stamped.length} runtime(s))"
end

main(ARGV)
