package dynacat

import "testing"

func TestConfigErrorPageAddr(t *testing.T) {
	cases := map[string]string{
		"server:\n  port: 8091\npages: []\n":                          ":8091",
		"server:\n  host: 0.0.0.0\n  port: \"9000\"\npages:\n  - [\n": "0.0.0.0:9000",
		"pages:\n  - [\n":            ":8080",
		"server:\n  port: ${PORT}\n": ":8080",
	}
	for in, want := range cases {
		if got := configErrorPageAddr([]byte(in)); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
