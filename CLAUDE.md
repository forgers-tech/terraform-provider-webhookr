# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working in this repository.

## Overview

A Terraform provider for Webhookr, written in Go (see `main.go`, `internal/`) using the
Terraform Plugin Framework. Examples live in `examples/`; the registry manifest is
`terraform-registry-manifest.json`.

## General rules

- **Never invent or assume any information.** If anything is unclear — a resource contract, a
  schema field, expected behavior — stop and ask before proceeding.
- **Everything must be in English** (code, comments, commit messages, PR titles/descriptions).
- Keep the provider schema and acceptance tests in sync with the Webhookr API it targets.

## Artifacts workflow (source of truth)

The **Artifacts repository (`webhookr-artifacts`)** is the long-term source of truth for
architecture, product decisions and operational knowledge. **Code is an implementation of the
artifacts, not the primary record.** Every meaningful change consumes knowledge from Artifacts
before coding and contributes knowledge back after.

### Before implementation

Before writing or modifying code:

1. Read the relevant Artifacts and understand the current architecture and product decisions.
2. Verify whether a governing artifact already exists — RFC, ADR, User Story, PMM, Security
   Matrix / Threat Model, or Operational guide / Runbook.
3. Do not introduce changes that contradict a documented decision. If a decision must change,
   update its artifact as part of the work (see below).
4. If the relevant documentation is missing or outdated, state the assumption you are making
   explicitly (label it `Assumption:`) before proceeding.

### After implementation

Once the change is complete and verified, apply the `artifact-reconciliation` standard
(forgers-tech `skills` plugin): if the change created or modified long-term knowledge —
architecture / ADR / RFC, security matrix / threat model, runbook or operational procedure,
infrastructure or deployment docs, API documentation, product behavior / User Story / PMM, or a
reusable engineering standard — update the corresponding artifact, explaining **why** the
decision exists rather than duplicating the implementation. Durable rationale lives in
`webhookr-artifacts/engineering` (or `marketing/`); operational docs live next to the code and
cross-link to the store artifact.

## Branch & working-tree hygiene

Start **and** finish every task on a clean tree, so the next agent never inherits a dirty
environment.

- **Start from a fresh `main`.** Run `git checkout main && git pull` before anything, then
  branch: `git checkout -b <type>/<short-slug>`. Never commit or push directly to `main`.
- **One branch → one PR → one concern**, following this repo's PR standard (always via PR,
  scoped to a single change, never mixing unrelated work).
- **Finish clean.** After the PR is merged, return the local repo to an updated `main` and drop
  the merged branch: `git checkout main && git pull && git branch -d <branch>`. Leave no stray
  branches, stashes, or uncommitted changes behind.
