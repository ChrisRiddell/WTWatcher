package modules

import (
	"errors"
	"net/netip"
	"os"
	"testing"
)

func TestResolveTargets_IPOnly(t *testing.T) {
	v4 := netip.MustParseAddr("192.168.1.1")
	v6 := netip.MustParseAddr("2606:4700:4700::1111")

	addrBoth := Address{
		Name: "Router",
		IPv4: &v4,
		IPv6: &v6,
	}

	targets, err := resolveTargets(addrBoth)
	if err != nil {
		t.Fatalf("resolveTargets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].host != "2606:4700:4700::1111" || targets[0].proto != "IPv6" {
		t.Errorf("target 0: want 2606:4700:4700::1111 (IPv6), got %v", targets[0])
	}
	if targets[1].host != "192.168.1.1" || targets[1].proto != "IPv4" {
		t.Errorf("target 1: want 192.168.1.1 (IPv4), got %v", targets[1])
	}
}

func TestResolveTargets_DomainProtocols(t *testing.T) {
	addrV4 := Address{
		Name:     "V4Site",
		Domain:   "example.com",
		Protocol: "IPv4",
	}
	targets, err := resolveTargets(addrV4)
	if err != nil {
		t.Fatalf("resolveTargets: %v", err)
	}
	if len(targets) != 1 || targets[0].host != "example.com" || targets[0].proto != "IPv4" {
		t.Errorf("expected 1 IPv4 target for example.com, got %v", targets)
	}

	addrV6 := Address{
		Name:     "V6Site",
		Domain:   "example.com",
		Protocol: "IPv6",
	}
	targets, err = resolveTargets(addrV6)
	if err != nil {
		t.Fatalf("resolveTargets: %v", err)
	}
	if len(targets) != 1 || targets[0].host != "example.com" || targets[0].proto != "IPv6" {
		t.Errorf("expected 1 IPv6 target for example.com, got %v", targets)
	}
}

func TestRoundTo2(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{12.3456, 12.35},
		{12.3444, 12.34},
		{0.0, 0.0},
		{1.1, 1.1},
		{100.999, 101.0},
	}
	for _, tc := range tests {
		got := roundTo2(tc.input)
		if got != tc.want {
			t.Errorf("roundTo2(%f): want %f, got %f", tc.input, tc.want, got)
		}
	}
}

func TestIsPermissionError(t *testing.T) {
	if !isPermissionError(os.ErrPermission) {
		t.Error("expected os.ErrPermission to be true")
	}
	if !isPermissionError(errors.New("operation not permitted")) {
		t.Error("expected operation not permitted to be true")
	}
	if !isPermissionError(errors.New("socket: permission denied")) {
		t.Error("expected permission denied to be true")
	}
	if isPermissionError(errors.New("connection refused")) {
		t.Error("expected connection refused to be false")
	}
	if isPermissionError(nil) {
		t.Error("expected nil to be false")
	}
}
