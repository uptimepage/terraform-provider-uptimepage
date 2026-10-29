package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

var manualCheck = CheckSpec{Type: CheckTypeManual}

// marshalKeys returns the top-level JSON object v encodes to.
func marshalKeys(t *testing.T, v any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return out
}

// The API refuses a PATCH naming both, so a group by id must not also carry
// the name key, not even as null.
func TestTargetUpdate_GroupIDTravelsAlone(t *testing.T) {
	got := marshalKeys(t, TargetUpdate{Check: manualCheck, GroupID: ptr("g1"), GroupName: ptr("dns")})
	if string(got["group_id"]) != `"g1"` {
		t.Errorf("group_id = %s", got["group_id"])
	}
	if _, has := got["group_name"]; has {
		t.Errorf("group_name sent beside group_id: %s", got["group_name"])
	}
}

func TestTargetUpdate_NoGroupClearsByName(t *testing.T) {
	got := marshalKeys(t, TargetUpdate{Check: manualCheck})
	if string(got["group_name"]) != "null" {
		t.Errorf("group_name = %s, want null", got["group_name"])
	}
	if _, has := got["group_id"]; has {
		t.Errorf("group_id sent: %s", got["group_id"])
	}
}

func TestTargetUpdate_KeepsTheRestOfTheBody(t *testing.T) {
	got := marshalKeys(t, TargetUpdate{
		Name:         "api",
		Check:        manualCheck,
		GroupName:    ptr("dns"),
		FiringPolicy: FiringPolicy{AlertConfirmations: 3, RegionPolicy: &RegionPolicy{Mode: RegionPolicyAll}},
	})
	if string(got["group_name"]) != `"dns"` || string(got["name"]) != `"api"` {
		t.Errorf("body = %v", got)
	}
	if string(got["alert_confirmations"]) != "3" || string(got["region_policy"]) != `"all"` {
		t.Errorf("firing policy lost: %v", got)
	}
}

func TestNewTarget_GroupIDOmittedWhenUnset(t *testing.T) {
	got := marshalKeys(t, NewTarget{Name: "api", Check: manualCheck})
	for _, k := range []string{"group_id", "group_name"} {
		if _, has := got[k]; has {
			t.Errorf("%s sent for a monitor with no group", k)
		}
	}
	got = marshalKeys(t, NewTarget{Name: "api", Check: manualCheck, GroupID: ptr("g1")})
	if string(got["group_id"]) != `"g1"` {
		t.Errorf("group_id = %s", got["group_id"])
	}
}

// A PATCH reads a null list as "keep", so a group whose channels and tags
// were all removed from config must send empty lists.
func TestUpdateGroup_SendsEmptyListsToClear(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/groups/g1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var in map[string]json.RawMessage
		_ = json.Unmarshal(body, &in)
		if string(in["alerts"]) != "[]" || string(in["tags"]) != "[]" {
			t.Errorf("body = %s", body)
		}
		_, _ = w.Write([]byte(`{"id":"g1","name":"dns","alert_as_one":true,"alerts":[],"renotify_interval_secs":0,"alert_again":false,"tags":[],"member_count":2,"write_source":"terraform","created_at":"2026-10-29T00:00:00Z","updated_at":"2026-10-29T00:00:00Z","open_incident_id":null}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "", srv.Client())
	got, err := c.UpdateGroup(context.Background(), "g1", MonitorGroupInput{Name: "dns", AlertAsOne: true})
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if !got.AlertAsOne || got.MemberCount != 2 || got.AlertAgain {
		t.Errorf("decoded = %+v", got)
	}
}

func TestListGroups_DecodesArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/groups" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":"g1","name":"dns","alert_as_one":false,"alerts":[{"channel_id":"c1"}],"renotify_interval_secs":3600,"alert_again":true,"tags":["p0"],"member_count":0}]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "", srv.Client())
	got, err := c.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(got) != 1 || got[0].Name != "dns" || len(got[0].Alerts) != 1 || got[0].Alerts[0].ChannelID != "c1" {
		t.Fatalf("decoded = %+v", got)
	}
}
