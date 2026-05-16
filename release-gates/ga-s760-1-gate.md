# Release Gate: ga-s760.1 hash versioning + silent rebaseline

Generated: 2026-05-16T15:45:05Z

Branch under gate: `deploy/ga-s760-1` from `fork/builder/ga-s760-1`

Source branch head: `501a6674205a`

Base: `origin/main` at `d24018969552`

Review bead: `ga-34pf`

Source bead: `ga-s760.1`

Note: `docs/PROJECT_MANIFEST.md` is not present in this repository or on
`origin/main`; this gate uses the release criteria from the deployer prompt.

## Gate Summary

| # | Criterion | Result | Evidence |
|---|-----------|--------|----------|
| 1 | Review PASS present | PASS | `ga-34pf` notes contain `Reviewer verdict: PASS` and `Reviewer verdict (gascity/reviewer, 2026-04-30): PASS`. |
| 2 | Acceptance criteria met | PASS | FR/NFR mapping below verified against code and validator tests. |
| 3 | Tests pass | PASS | Targeted validator/regression command PASS; `make test-fast-parallel` PASS; `go vet ./...` PASS; `go build ./...` PASS. |
| 4 | No high-severity review findings open | PASS | Review notes list no HIGH findings; only accepted non-blocking coverage notes and a previously fixed gate failure. |
| 5 | Final branch is clean | PASS | `git status --short --branch` was clean before gate generation; this gate file is committed as the release-gate commit and final clean status is rechecked before push. |
| 6 | Branch diverges cleanly from main | PASS | `git rev-list --left-right --count origin/main...HEAD` returned `0 2`; `git merge-tree $(git merge-base HEAD origin/main) HEAD origin/main` produced no conflicts. |

## Change Set

| Commit | Purpose |
|--------|---------|
| `1ea9e069b` | Validator tests for versioned fingerprint output, version mismatch classification, silent rebaseline, and same-version real drift. |
| `501a66742` | Implementation: version stored fingerprints and silently rebaseline legacy/version-mismatched session hashes. |

Touched files:

- `internal/runtime/fingerprint.go`
- `internal/runtime/fingerprint_test.go`
- `cmd/gc/session_reconciler.go`
- `cmd/gc/session_reconciler_test.go`
- `cmd/gc/session_reconciler_trace_types.go`

## Acceptance Criteria Evidence

| Requirement | Result | Evidence |
|-------------|--------|----------|
| FR-1: legacy unversioned stored hash must not drain | PASS | `runtime.IsLegacyOrMismatchedVersion` classifies bare hashes as rebaseline candidates; `TestReconcilerSilentRebaselineOnLegacyHash` asserts no `SessionDraining` event and no drain. |
| FR-2: version-mismatched stored hash must not drain | PASS | `runtime.IsLegacyOrMismatchedVersion` classifies `v0:`/other version prefixes as mismatched; `TestReconcilerSilentRebaselineOnVersionMismatch` asserts no drain. |
| FR-3: rebaseline writes four metadata fields atomically | PASS | `silentRebaselineSessionHashes` builds one `MetadataPatch` for `started_config_hash`, `started_live_hash`, `live_hash`, and `core_hash_breakdown`, then calls `SetMetadataBatch`; both rebaseline tests assert the fields are refreshed. |
| FR-4: same-version real drift still drains | PASS | `TestReconcilerStillDrainsOnSameVersionRealDrift` asserts existing config-drift drain behavior remains active for same-version hash drift. |
| FR-5: newly committed hashes are versioned | PASS | `ConfigFingerprint`, `CoreFingerprint`, and `LiveFingerprint` return `FingerprintVersion + ":" + sha256`; `TestFingerprintVersionedOutputFormat` covers all three. |
| FR-6: breakdown version tag | PASS | Source bead explicitly defers structural breakdown versioning to MF-A (`ga-s760.2`); this bead rebaselines `core_hash_breakdown` to the current breakdown when legacy/mismatch hashes are detected. |
| FR-7: single version constant | PASS | `runtime.FingerprintVersion` is the only source of the current `v1` namespace; `TestFingerprintVersionedOutputFormat` enforces shape and use. |
| NFR-1: no extra reconciler I/O | PASS | Version classification is string inspection of already-loaded metadata before the existing hash drift path. |
| NFR-2: existing fingerprint callers remain comparable | PASS | Existing fingerprint tests continue to compare outputs from the same versioned helpers; full fast unit baseline passed. |
| NFR-3: version bump is one-line constant edit | PASS | Current version is isolated to `internal/runtime/fingerprint.go` as `const FingerprintVersion = "v1"`. |
| NFR-4: no regex in hot path | PASS | `IsVersionMismatchedHash` uses direct string/digit scanning. |

## Test Evidence

Commands run on `deploy/ga-s760-1`:

```text
go test ./internal/runtime ./cmd/gc -run 'TestFingerprintVersionedOutputFormat|TestIsLegacyOrMismatchedVersion|TestReconcilerSilentRebaseline|TestReconcilerStillDrainsOnSameVersionRealDrift|TestPhase0CanonicalMetadata_NamedMaterializationWritesNamedOriginWithoutLegacyManualFlag' -count=1
ok  	github.com/gastownhall/gascity/internal/runtime	0.004s
ok  	github.com/gastownhall/gascity/cmd/gc	0.127s

make test-fast-parallel
All fast jobs passed

go vet ./...
PASS

go build ./...
PASS
```

## Review Findings

Unresolved HIGH findings: 0

The review notes call out only non-blocking accepted coverage gaps:

- live-only drift branch and asleep-named branch mirror the tested core branch;
- trace outcome split is deterministic but not directly asserted;
- hash truncation helper is used by exercised code paths but not directly unit-tested.

## Branch Evidence

```text
git rev-list --left-right --count origin/main...HEAD
0	2

git merge-tree $(git merge-base HEAD origin/main) HEAD origin/main
<no output>
```
