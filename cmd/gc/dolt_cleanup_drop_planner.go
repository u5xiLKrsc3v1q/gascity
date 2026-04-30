package main

import "strings"

// defaultStaleDatabasePrefixes mirrors
// /home/jaword/projects/beads/cmd/bd/dolt.go:staleDatabasePrefixes — the
// list of name prefixes that identify test/agent databases left behind by
// interrupted runs. The lists must converge (be-hjj-3 syncs the beads side).
//
// Convention:
//   - testdb_*: BEADS_TEST_MODE=1 FNV hash of temp paths
//   - doctest_*: doctor test helpers
//   - doctortest_*: doctor test helpers
//   - beads_pt*: orchestrator patrol_helpers_test.go random prefixes
//   - beads_vr*: orchestrator mail/router_test.go random prefixes
//   - beads_t[0-9a-f]*: protocol test random prefixes (t + 8 hex chars)
var defaultStaleDatabasePrefixes = []string{
	"testdb_", "doctest_", "doctortest_", "beads_pt", "beads_vr", "beads_t",
}

// systemDatabaseNames are the Dolt/MySQL system databases that SHOW
// DATABASES surfaces. The planner never targets these even if a stale
// prefix accidentally matches.
var systemDatabaseNames = map[string]bool{
	"information_schema": true,
	"mysql":              true,
	"performance_schema": true,
	"sys":                true,
	"dolt_cluster":       true,
}

// DoltDropPlan classifies a SHOW DATABASES result into to-drop, protected,
// and explicitly-skipped sets. Pure logic; no I/O.
type DoltDropPlan struct {
	// ToDrop is the set of DB names whose prefix matches a stale entry and
	// which are not protected by the rig registry or system list.
	ToDrop []string
	// Protected is the set of DB names that match a stale prefix but are
	// shielded by the rig-protection list. They are also recorded in
	// Skipped with reason="rig-protected" for human-readable output.
	Protected []string
	// Skipped records every name the planner considered but did not put on
	// the drop list, with a short reason ("system", "rig-protected",
	// "no-prefix-match"). Useful for diagnostic output.
	Skipped []DoltDropSkip
}

// DoltDropSkip is a single planner-skipped database with the reason.
type DoltDropSkip struct {
	Name   string
	Reason string
}

// planDoltDrops classifies the names returned by SHOW DATABASES against the
// stale-prefix list and the rig-protection list. The protection check wins
// over the stale-prefix match: a registered rig DB is never a drop target,
// even if its name happens to start with a known stale prefix.
func planDoltDrops(allDBs, stalePrefixes, protectedNames []string) DoltDropPlan {
	protected := map[string]bool{}
	for _, p := range protectedNames {
		protected[p] = true
	}

	plan := DoltDropPlan{}
	for _, name := range allDBs {
		if systemDatabaseNames[name] {
			plan.Skipped = append(plan.Skipped, DoltDropSkip{Name: name, Reason: "system"})
			continue
		}
		if !hasAnyPrefix(name, stalePrefixes) {
			plan.Skipped = append(plan.Skipped, DoltDropSkip{Name: name, Reason: "no-prefix-match"})
			continue
		}
		if protected[name] {
			plan.Protected = append(plan.Protected, name)
			plan.Skipped = append(plan.Skipped, DoltDropSkip{Name: name, Reason: "rig-protected"})
			continue
		}
		plan.ToDrop = append(plan.ToDrop, name)
	}
	return plan
}

func hasAnyPrefix(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
