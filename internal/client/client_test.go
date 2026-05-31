package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newHTTPTarget() NewTarget {
	return NewTarget{
		Name:     "api prod",
		Interval: 60,
		Enabled:  true,
		Tags:     []string{"prod"},
		Check: CheckSpec{
			Type: CheckTypeHTTP,
			HTTP: &HTTPCheck{
				URL:            "https://example.com/healthz",
				Method:         "GET",
				Timeout:        5000,
				MaxRedirects:   5,
				ExpectedStatus: ExpectedStatus{Kind: StatusKindExact, Exact: 200},
				Headers:        map[string]string{},
				VerifyTLS:      true,
			},
		},
	}
}

func TestCreateTarget_SendsAuthAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sm_live_test" {
			t.Errorf("auth header = %q, want Bearer sm_live_test", got)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/targets" {
			t.Errorf("got %s %s, want POST /api/v1/targets", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var in NewTarget
		if err := json.Unmarshal(body, &in); err != nil {
			t.Fatalf("server could not decode body: %v", err)
		}
		if in.Name != "api prod" {
			t.Errorf("decoded name = %q", in.Name)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"01h7","name":"api prod","check":{"type":"http","url":"https://example.com/healthz","method":"GET","timeout":5000,"follow_redirects":false,"max_redirects":5,"expected_status":{"kind":"exact","value":200},"expected_body_contains":null,"headers":{},"body":null,"verify_tls":true,"basic_auth":null,"bearer_token":null},"interval":60,"enabled":true,"tags":["prod"],"alerts":[],"group_name":null,"owner_user_id":null}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "sm_live_test", "", srv.Client())
	got, err := c.CreateTarget(context.Background(), newHTTPTarget())
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	if got.ID != "01h7" {
		t.Errorf("id = %q, want 01h7", got.ID)
	}
	if got.Check.Type != CheckTypeHTTP || got.Check.HTTP == nil {
		t.Fatalf("check not decoded as http: %+v", got.Check)
	}
	if got.Check.HTTP.ExpectedStatus.Exact != 200 {
		t.Errorf("expected_status.exact = %d, want 200", got.Check.HTTP.ExpectedStatus.Exact)
	}
}

func TestDo_DecodesErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"INVALID_URL_SCHEME","message":"url scheme 'ftp' not allowed","field":"check.url","trace_id":null}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "t", "", srv.Client())
	_, err := c.GetTarget(context.Background(), "x")
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if ae.Code != "INVALID_URL_SCHEME" || ae.Field != "check.url" || ae.Status != 400 {
		t.Errorf("APIError = %+v", ae)
	}
}

func TestGetTarget_404IsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"TARGET_NOT_FOUND","message":"target not found"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "t", "", srv.Client())
	_, err := c.GetTarget(context.Background(), "missing")
	if !IsNotFound(err) {
		t.Fatalf("IsNotFound = false, err = %v", err)
	}
}

func TestDeleteTarget_204NoBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, "t", "", srv.Client())
	if err := c.DeleteTarget(context.Background(), "id"); err != nil {
		t.Fatalf("DeleteTarget: %v", err)
	}
}

func TestDo_ContextCancelAborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the call

	c := New(srv.URL, "t", "", srv.Client())
	_, err := c.GetTarget(ctx, "id")
	if err == nil {
		t.Fatal("want error from cancelled context, got nil")
	}
}

func TestOrgHeader(t *testing.T) {
	newSrv := func(check func(*http.Request)) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			check(r)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"x","name":"api prod","check":{"type":"http","url":"https://example.com/healthz","method":"GET","timeout":5000,"follow_redirects":false,"max_redirects":5,"expected_status":{"kind":"exact","value":200},"expected_body_contains":null,"headers":{},"body":null,"verify_tls":true,"basic_auth":null,"bearer_token":null},"interval":60,"enabled":true,"tags":[],"alerts":[],"group_name":null,"owner_user_id":null}`))
		}))
	}
	t.Run("sent when org set", func(t *testing.T) {
		srv := newSrv(func(r *http.Request) {
			if got := r.Header.Get("X-Uptimepage-Org"); got != "acme" {
				t.Errorf("X-Uptimepage-Org = %q, want acme", got)
			}
		})
		defer srv.Close()
		if _, err := New(srv.URL, "t", "acme", srv.Client()).CreateTarget(context.Background(), newHTTPTarget()); err != nil {
			t.Fatalf("CreateTarget: %v", err)
		}
	})
	t.Run("absent when org empty", func(t *testing.T) {
		srv := newSrv(func(r *http.Request) {
			if _, ok := r.Header["X-Uptimepage-Org"]; ok {
				t.Errorf("X-Uptimepage-Org present, want absent")
			}
		})
		defer srv.Close()
		if _, err := New(srv.URL, "t", "", srv.Client()).CreateTarget(context.Background(), newHTTPTarget()); err != nil {
			t.Fatalf("CreateTarget: %v", err)
		}
	})
}
