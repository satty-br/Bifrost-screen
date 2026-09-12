package mancer

import "testing"

func TestClampTemp(t *testing.T) {
	cases := []struct {
		in   float64
		want byte
	}{
		{-10, 0},
		{0, 0},
		{45, 45},
		{45.4, 45},
		{45.5, 46},
		{99, 99},
		{150, 99},
	}
	for _, c := range cases {
		if got := clampTemp(c.in); got != c.want {
			t.Errorf("clampTemp(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestMonitorInitialState(t *testing.T) {
	m := NewMonitor()
	if m.Connected() {
		t.Error("um monitor novo não deveria estar conectado")
	}
	if m.LastError() != "" {
		t.Errorf("um monitor novo não deveria ter erro, veio %q", m.LastError())
	}
}
