package main

import (
	"strings"
	"testing"
	"time"
)

func TestPrintableASCII(t *testing.T) {
	for c := byte(32); c <= 126; c++ {
		p, err := plan(string(c), false)
		if err != nil {
			t.Fatal(err)
		}
		s := p[0]
		restored := s.key
		if s.shift {
			if s.key >= 'a' && s.key <= 'z' {
				restored -= 32
			} else {
				i := strings.IndexByte(unshifted, s.key)
				if i < 0 {
					t.Fatal(s)
				}
				restored = shifted[i]
			}
		}
		if restored != c {
			t.Fatalf("%q -> %+v -> %q", c, s, restored)
		}
	}
}

func TestModifierRelease(t *testing.T) {
	checks, waits := 0, 0
	ok := waitModifiersReleased(func() bool { checks++; return checks < 4 }, func(d time.Duration) bool {
		if d != 10*time.Millisecond {
			t.Fatalf("unexpected poll interval %v", d)
		}
		waits++
		return true
	})
	if !ok || waits != 3 {
		t.Fatalf("release: ok=%v waits=%d", ok, waits)
	}
}
func TestModifierReleaseCancelAndTimeout(t *testing.T) {
	if waitModifiersReleased(func() bool { return true }, func(time.Duration) bool { return false }) {
		t.Fatal("cancellation ignored")
	}
	waits := 0
	if waitModifiersReleased(func() bool { return true }, func(time.Duration) bool { waits++; return true }) {
		t.Fatal("stuck modifiers accepted")
	}
	if waits != 1000 {
		t.Fatalf("unexpected timeout: %d polls", waits)
	}
}
func TestLineEndings(t *testing.T) {
	p, err := plan("a\r\nb\rc\n\t", true)
	if err != nil {
		t.Fatal(err)
	}
	var text []byte
	for _, s := range p {
		text = append(text, s.key)
	}
	if string(text) != "a\nb\nc\n\t" {
		t.Fatal(string(text))
	}
}
func TestReject(t *testing.T) {
	for _, text := range []string{"", "hello я", "😀", "\x00", "\x1b", "a\n", "a\t"} {
		if _, err := plan(text, false); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}

func TestZeroIntervalCancellation(t *testing.T) {
	cancel := make(chan struct{})
	if !waitInput(0, cancel, func() bool { return false }) {
		t.Fatal("zero interval rejected")
	}
	if waitInput(0, cancel, func() bool { return true }) {
		t.Fatal("Esc ignored at zero interval")
	}
	close(cancel)
	if waitInput(0, cancel, func() bool { return false }) {
		t.Fatal("stop ignored at zero interval")
	}
}
