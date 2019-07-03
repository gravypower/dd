package dd

import (
	"testing"
)

func TestCrypto(t *testing.T) {
	var s, expected string

	// test phone sig
	hs1 := newHubSignature([]byte("AjXEy8OcGOrwwEdQ"))
	s = hs1.Update(1520743556636, "hNjUL66TaJE8FptPOHcYfw==")
	expected = "Xk+51cz6/a+J5cKHhJetcMBs2fCB5nEh0A9oEg2REzk="
	if s != expected {
		t.Errorf("got \"%s\", expected \"%s\"", s, expected)
	}

	// test session sig
	hs2 := newHubSignature([]byte("GznHzaWnOwrQx3KJA3U8Ly"))
	s = hs2.Update(1520743556636, "hNjUL66TaJE8FptPOHcYfw==")
	expected = "ohiskyORKqOGorvv5gyJjIL+p4y2Zg3XN8iDlbU2C84="
	if s != expected {
		t.Errorf("got \"%s\", expected \"%s\"", s, expected)
	}

	// test phone sig again: replay should be the same
	s = hs1.Update(1520743556636, "hNjUL66TaJE8FptPOHcYfw==")
	expected = "Xk+51cz6/a+J5cKHhJetcMBs2fCB5nEh0A9oEg2REzk="
	if s != expected {
		t.Errorf("got \"%s\", expected \"%s\" (replay should match)", s, expected)
	}
}
