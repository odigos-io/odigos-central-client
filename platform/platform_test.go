package platform

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in      string
		want    Type
		wantErr bool
	}{
		{"K8S", K8s, false},
		{"K8s", K8s, false},
		{"k8s", K8s, false},
		{"kubernetes", K8s, false},
		{"Kubernetes", K8s, false},
		{"VM", Vm, false},
		{"Vm", Vm, false},
		{"vm", Vm, false},
		// Mixed casing and surrounding whitespace must normalize.
		{"k8S", K8s, false},
		{"KuBeRnEtEs", K8s, false},
		{"  k8s  ", K8s, false},
		{"\tVM\n", Vm, false},
		{"", "", true},
		{"unknown", "", true},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("Parse(%q) expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Parse(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
