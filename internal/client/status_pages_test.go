package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestCreateStatusPage_SendsIdentityOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/status-pages" {
			t.Errorf("got %s %s, want POST /api/v1/status-pages", r.Method, r.URL.Path)
		}
		var in map[string]any
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &in)
		if _, has := in["branding"]; has {
			t.Errorf("create body must not carry branding: %s", body)
		}
		if in["slug"] != "acme" {
			t.Errorf("slug = %v", in["slug"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"p1","slug":"acme","name":"Acme","enabled":true,"public_style":"default","show_powered_by":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "", srv.Client())
	got, err := c.CreateStatusPage(context.Background(), NewStatusPage{Slug: "acme", Name: "Acme", Enabled: true})
	if err != nil {
		t.Fatalf("CreateStatusPage: %v", err)
	}
	if got.ID != "p1" || got.PublicStyle != "default" || !got.ShowPoweredBy {
		t.Errorf("decoded page = %+v", got)
	}
}

func TestUpdateStatusPage_NilBrandingMarshalsNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/status-pages/p1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		// A nil display name must travel as JSON null (clear), not be omitted.
		var raw struct {
			Branding map[string]json.RawMessage `json:"branding"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &raw)
		v, has := raw.Branding["public_display_name"]
		if !has || string(v) != "null" {
			t.Errorf("public_display_name = %s (has=%v), want null", v, has)
		}
		if got := string(raw.Branding["public_brand_color"]); got != `"#0a0"` {
			t.Errorf("public_brand_color = %s, want \"#0a0\"", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"p1","slug":"acme","name":"Acme","enabled":false,"public_brand_color":"#0a0","public_style":"dark","show_powered_by":false}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "", srv.Client())
	got, err := c.UpdateStatusPage(context.Background(), "p1", StatusPageUpdate{
		Name: "Acme", Slug: "acme", Enabled: false,
		Branding: StatusBranding{
			PublicBrandColor: ptr("#0a0"),
			PublicStyle:      ptr("dark"),
		},
	})
	if err != nil {
		t.Fatalf("UpdateStatusPage: %v", err)
	}
	if got.PublicStyle != "dark" || got.PublicBrandColor == nil || *got.PublicBrandColor != "#0a0" {
		t.Errorf("decoded = %+v", got)
	}
}

func TestAddComponent_PostsTargetAnd204(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/status-pages/p1/components" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		var in NewStatusPageComponent
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &in)
		if in.TargetID != "t1" || in.SortOrder != 3 {
			t.Errorf("decoded = %+v", in)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "", srv.Client())
	err := c.AddComponent(context.Background(), "p1", NewStatusPageComponent{TargetID: "t1", SortOrder: 3})
	if err != nil {
		t.Fatalf("AddComponent: %v", err)
	}
}

func TestAddComponent_ConflictSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"COMPONENT_ALREADY_ON_PAGE","message":"already on page"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "", srv.Client())
	err := c.AddComponent(context.Background(), "p1", NewStatusPageComponent{TargetID: "t1"})
	ae, ok := err.(*APIError)
	if !ok || ae.Code != "COMPONENT_ALREADY_ON_PAGE" {
		t.Fatalf("want COMPONENT_ALREADY_ON_PAGE APIError, got %v", err)
	}
}

func TestListComponents_DecodesArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"target_id":"t1","monitor_name":"API","public_name":"API (public)","public_description":null,"public_group":"Core","sort_order":0}]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "", srv.Client())
	got, err := c.ListComponents(context.Background(), "p1")
	if err != nil {
		t.Fatalf("ListComponents: %v", err)
	}
	if len(got) != 1 || got[0].TargetID != "t1" || got[0].MonitorName != "API" {
		t.Fatalf("decoded = %+v", got)
	}
	if got[0].PublicName == nil || *got[0].PublicName != "API (public)" {
		t.Errorf("public_name = %v", got[0].PublicName)
	}
	if got[0].PublicDescription != nil {
		t.Errorf("public_description = %v, want nil", got[0].PublicDescription)
	}
}
