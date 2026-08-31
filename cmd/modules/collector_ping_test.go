package modules

import (
	"errors"
	"net/netip"
	"os"
	"testing"
	"time"
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

func TestFilterAnomalyRTTs(t *testing.T) {
	ms := func(n int64) time.Duration { return time.Duration(n) * time.Millisecond }
	threshold := int64(2000) // 2 000 ms default

	t.Run("all normal — no filter", func(t *testing.T) {
		rtts := []time.Duration{ms(10), ms(12), ms(11), ms(13)}
		got := filterAnomalyRTTs(rtts, threshold)
		if got != nil {
			t.Errorf("expected nil (no outliers), got %v", got)
		}
	})

	t.Run("single extreme spike is removed", func(t *testing.T) {
		// Three normal pings + one absurd spike typical of OS jitter.
		rtts := []time.Duration{ms(12), ms(11), ms(13), ms(25000)}
		got := filterAnomalyRTTs(rtts, threshold)
		if got == nil {
			t.Fatal("expected filtered slice, got nil")
		}
		for _, rtt := range got {
			if rtt >= ms(25000) {
				t.Errorf("spike %v should have been removed", rtt)
			}
		}
		if len(got) < 2 {
			t.Errorf("expected at least 2 clean RTTs, got %d", len(got))
		}
	})

	t.Run("all samples are spikes — returns nil (too few clean)", func(t *testing.T) {
		// With all packets spiked, filtering leaves < 2 samples.
		rtts := []time.Duration{ms(30000), ms(40000)}
		got := filterAnomalyRTTs(rtts, threshold)
		if got != nil {
			t.Errorf("expected nil when all are outliers, got %v", got)
		}
	})

	t.Run("single element — returns nil", func(t *testing.T) {
		got := filterAnomalyRTTs([]time.Duration{ms(10)}, threshold)
		if got != nil {
			t.Errorf("expected nil for single-element input, got %v", got)
		}
	})

	t.Run("empty — returns nil", func(t *testing.T) {
		got := filterAnomalyRTTs(nil, threshold)
		if got != nil {
			t.Errorf("expected nil for empty input, got %v", got)
		}
	})

	t.Run("two normal pings — no filter (too few to have outliers)", func(t *testing.T) {
		rtts := []time.Duration{ms(10), ms(12)}
		got := filterAnomalyRTTs(rtts, threshold)
		if got != nil {
			t.Errorf("expected nil for two identical-ish pings, got %v", got)
		}
	})

	t.Run("realistic 4-packet run with one spike", func(t *testing.T) {
		// Typical scenario: 3 good pings + 1 huge OS jitter spike.
		rtts := []time.Duration{ms(8), ms(9), ms(10), ms(35000)}
		got := filterAnomalyRTTs(rtts, threshold)
		if got == nil {
			t.Fatal("expected filtered slice for 4-packet run with spike")
		}
		if len(got) != 3 {
			t.Errorf("expected 3 clean RTTs, got %d: %v", len(got), got)
		}
	})

	t.Run("4-packet run with spike just above threshold", func(t *testing.T) {
		// Three ~30ms pings + one 2500ms spike above the 2000ms threshold.
		rtts := []time.Duration{ms(30), ms(31), ms(29), ms(2500)}
		got := filterAnomalyRTTs(rtts, threshold)
		if got == nil {
			t.Fatal("expected filtered slice for 2500ms spike")
		}
		if len(got) != 3 {
			t.Errorf("expected 3 clean RTTs, got %d: %v", len(got), got)
		}
	})
}
