package main

import (
	"encoding/json"
	"testing"

	mirastack "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
)

func TestInfo_HasPerActionIntents(t *testing.T) {
	p := &QueryVLogsPlugin{}
	info := p.Info()

	if info.Version != "0.2.0" {
		t.Errorf("expected version 0.2.0, got %s", info.Version)
	}

	for _, action := range info.Actions {
		if len(action.Intents) == 0 {
			t.Errorf("action %q has no per-action intents", action.ID)
		}
	}
}

func TestInfo_HasPromptTemplates(t *testing.T) {
	p := &QueryVLogsPlugin{}
	info := p.Info()

	if len(info.PromptTemplates) == 0 {
		t.Fatal("expected at least one PromptTemplate")
	}
	if info.PromptTemplates[0].Name != "query_vlogs_guide" {
		t.Errorf("expected template name query_vlogs_guide, got %s", info.PromptTemplates[0].Name)
	}
}

func TestInfo_PluginIntentsExpanded(t *testing.T) {
	p := &QueryVLogsPlugin{}
	info := p.Info()

	if len(info.Intents) < 5 {
		t.Errorf("expected >=5 plugin-level intents, got %d", len(info.Intents))
	}
}

func TestEnrichLogsOutput_BasicFields(t *testing.T) {
	out := enrichLogsOutput("query", `{"msg":"test"}`)

	if out["action"] != "query" {
		t.Errorf("expected action=query, got %v", out["action"])
	}
}

func TestEnrichLogsOutput_CountsNDJSONLines(t *testing.T) {
	ndjson := "{\"msg\":\"line1\"}\n{\"msg\":\"line2\"}\n{\"msg\":\"line3\"}\n"
	out := enrichLogsOutput("query", ndjson)

	if out["result_count"] != 3 {
		t.Errorf("expected result_count=3, got %v", out["result_count"])
	}
}

func TestEnrichLogsOutput_JSONArray(t *testing.T) {
	raw := `["field_a","field_b","field_c"]`
	out := enrichLogsOutput("field_names", raw)

	if out["result_count"] != 3 {
		t.Errorf("expected result_count=3, got %v", out["result_count"])
	}
}

func TestEnrichLogsOutput_Truncation(t *testing.T) {
	long := make([]byte, 33000)
	for i := range long {
		long[i] = 'x'
	}
	out := enrichLogsOutput("query", string(long))

	if out["truncated"] != true {
		t.Error("expected truncated=true for oversized result")
	}
}

func TestEnrichLogsOutput_JSONMarshalable(t *testing.T) {
	out := enrichLogsOutput("stats", `{"count":42}`)

	_, err := json.Marshal(out)
	if err != nil {
		t.Errorf("enriched output not JSON-serializable: %v", err)
	}
}

func TestInfo_ActionDescriptionsEnriched(t *testing.T) {
	p := &QueryVLogsPlugin{}
	info := p.Info()

	for _, action := range info.Actions {
		if len(action.Description) < 50 {
			t.Errorf("action %q description too short (%d chars)", action.ID, len(action.Description))
		}
	}
}

func TestInfo_DeleteStreamAction_AdminPermission(t *testing.T) {
	p := &QueryVLogsPlugin{}
	info := p.Info()

	var found bool
	for _, action := range info.Actions {
		if action.ID == "delete_stream" {
			found = true
			if action.Permission != mirastack.PermissionAdmin {
				t.Errorf("delete_stream should have ADMIN permission, got %v", action.Permission)
			}
			if len(action.Intents) == 0 {
				t.Error("delete_stream should have per-action intents")
			}
			hasMatch := false
			for _, p := range action.InputParams {
				if p.Name == "match" && p.Required {
					hasMatch = true
				}
			}
			if !hasMatch {
				t.Error("delete_stream should have required 'match' input param")
			}
		}
	}
	if !found {
		t.Fatal("delete_stream action not found in Info()")
	}
}

func TestInfo_PluginPermissionsIncludeAdmin(t *testing.T) {
	p := &QueryVLogsPlugin{}
	info := p.Info()

	hasAdmin := false
	for _, perm := range info.Permissions {
		if perm == mirastack.PermissionAdmin {
			hasAdmin = true
		}
	}
	if !hasAdmin {
		t.Error("plugin permissions should include ADMIN")
	}
}

func TestActionDeleteStream_RequiresMatch(t *testing.T) {
	p := &QueryVLogsPlugin{}
	_, err := p.actionDeleteStream(nil, map[string]string{}, nil)
	if err == nil {
		t.Error("expected error when match is empty")
	}
}
