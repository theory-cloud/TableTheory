package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func TestRequireOrgBindingRejectsCrossOrgAndUnbound(t *testing.T) {
	called := false
	next := func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}
	handler := requireOrgBinding(next)

	requestFor := func(orgID string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID+"/users", nil)
		return mux.SetURLVars(r, map[string]string{"org_id": orgID})
	}
	withBoundOrg := func(r *http.Request, orgID string) *http.Request {
		return r.WithContext(context.WithValue(r.Context(), "org_id", orgID))
	}

	// An unbound caller is refused outright.
	rec := httptest.NewRecorder()
	handler(rec, requestFor("org#alpha"))
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, called)

	// A caller bound to a different organization is refused without confirming
	// the other organization exists.
	rec = httptest.NewRecorder()
	handler(rec, withBoundOrg(requestFor("org#beta"), "org#alpha"))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.False(t, called)

	// A caller bound to the requested organization is served.
	rec = httptest.NewRecorder()
	handler(rec, withBoundOrg(requestFor("org#alpha"), "org#alpha"))
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, called)
}

func TestRouterEnforcesOrgBindingOnEveryOrgRoute(t *testing.T) {
	router := NewRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/organizations/org%23beta/users", nil)
	req = req.WithContext(context.WithValue(req.Context(), "org_id", "org#alpha"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code, "a cross-organization URL must be denied before the handler runs")
}
