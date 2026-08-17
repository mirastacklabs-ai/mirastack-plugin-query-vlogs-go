package main

import "testing"

// Every service-name parameter must declare the catalog value source. Without it
// the engine's referential validation never fires, so a near-miss the LLM
// produced ("upi switch" for "upi-switch") is dispatched verbatim and the action
// answers with empty data — indistinguishable from a real outage.
func TestServiceParamsDeclareCatalogValueSource(t *testing.T) {
	info := (&QueryVLogsPlugin{}).Info()
	seen := 0
	for _, action := range info.Actions {
		for _, param := range action.InputParams {
			if param.Name != "service" {
				continue
			}
			seen++
			if param.ValueSource != catalogServicesValueSource {
				t.Errorf("action %q param %q ValueSource = %q, want %q",
					action.ID, param.Name, param.ValueSource, catalogServicesValueSource)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no service parameters found — this guard would silently pass forever")
	}
}
