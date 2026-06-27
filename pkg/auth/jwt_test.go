package auth

import "testing"

func newTestManager() *Manager {
	return NewManager("test-secret", 60, 720)
}

func TestOwnerTokenRoundTrip(t *testing.T) {
	m := newTestManager()
	access, refresh, err := m.GenerateOwnerTokens("ou-1", "owner-1")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	claims, err := m.ParseOwner(access, TypeAccess)
	if err != nil {
		t.Fatalf("parse access: %v", err)
	}
	if claims.OwnerUserID != "ou-1" || claims.OwnerID != "owner-1" {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	if _, err := m.ParseOwner(refresh, TypeRefresh); err != nil {
		t.Fatalf("parse refresh: %v", err)
	}
}

func TestTenantTokenRoundTrip(t *testing.T) {
	m := newTestManager()
	access, _, err := m.GenerateTenantTokens("t-1", "owner-1")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	claims, err := m.ParseTenant(access, TypeAccess)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.TenantID != "t-1" || claims.OwnerID != "owner-1" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestWrongTokenTypeRejected(t *testing.T) {
	m := newTestManager()
	access, refresh, _ := m.GenerateOwnerTokens("ou-1", "owner-1")

	if _, err := m.ParseOwner(access, TypeRefresh); err == nil {
		t.Fatal("access token accepted as refresh")
	}
	if _, err := m.ParseOwner(refresh, TypeAccess); err == nil {
		t.Fatal("refresh token accepted as access")
	}
}

func TestCrossActorTokenRejected(t *testing.T) {
	m := newTestManager()
	ownerAccess, _, _ := m.GenerateOwnerTokens("ou-1", "owner-1")
	tenantAccess, _, _ := m.GenerateTenantTokens("t-1", "owner-1")

	// Owner token must not parse as a valid tenant token (no tenant_id).
	if _, err := m.ParseTenant(ownerAccess, TypeAccess); err == nil {
		t.Fatal("owner token accepted as tenant token")
	}
	// Tenant token must not parse as a valid owner token (no owner_user_id).
	if _, err := m.ParseOwner(tenantAccess, TypeAccess); err == nil {
		t.Fatal("tenant token accepted as owner token")
	}
}

func TestTamperedTokenRejected(t *testing.T) {
	m := newTestManager()
	access, _, _ := m.GenerateOwnerTokens("ou-1", "owner-1")

	other := NewManager("different-secret", 60, 720)
	if _, err := other.ParseOwner(access, TypeAccess); err == nil {
		t.Fatal("token verified under a different secret")
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	m := NewManager("test-secret", 0, 0) // already expired
	access, _, _ := m.GenerateOwnerTokens("ou-1", "owner-1")
	if _, err := m.ParseOwner(access, TypeAccess); err == nil {
		t.Fatal("expired token accepted")
	}
}
