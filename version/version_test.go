package version

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in      string
		want    Version
		wantErr bool
	}{
		{"v1.20", Version{1, 20}, false},
		{"v1.20.0", Version{1, 20}, false},
		{"v1.22.0-rc0", Version{1, 22}, false},
		{"1.20", Version{1, 20}, false},
		{"1.20.5", Version{1, 20}, false},
		{"v0.1", Version{0, 1}, false},
		{"v10.300", Version{10, 300}, false},
		{"", Version{}, true},
		{"vfoo", Version{}, true},
		{"v1", Version{}, true},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("Parse(%q) expected error, got %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Parse(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestCompareAndOrdering(t *testing.T) {
	cases := []struct {
		a, b Version
		cmp  int
	}{
		{Version{1, 20}, Version{1, 20}, 0},
		{Version{1, 20}, Version{1, 21}, -1},
		{Version{1, 21}, Version{1, 20}, 1},
		{Version{1, 20}, Version{2, 0}, -1},
		{Version{2, 0}, Version{1, 99}, 1},
		{Version{0, 1}, Version{1, 0}, -1},
	}
	for _, c := range cases {
		if got := c.a.Compare(c.b); got != c.cmp {
			t.Errorf("(%v).Compare(%v) = %d, want %d", c.a, c.b, got, c.cmp)
		}
		gte := c.a.GTE(c.b)
		wantGTE := c.cmp >= 0
		if gte != wantGTE {
			t.Errorf("(%v).GTE(%v) = %v, want %v", c.a, c.b, gte, wantGTE)
		}
		lt := c.a.LT(c.b)
		wantLT := c.cmp < 0
		if lt != wantLT {
			t.Errorf("(%v).LT(%v) = %v, want %v", c.a, c.b, lt, wantLT)
		}
	}
}

func TestString(t *testing.T) {
	if got := (Version{1, 20}).String(); got != "v1.20" {
		t.Errorf("String() = %q, want v1.20", got)
	}
	if got := (Version{0, 1}).String(); got != "v0.1" {
		t.Errorf("String() = %q, want v0.1", got)
	}
}

func TestMustParsePanicsOnBadInput(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustParse should panic on bad input")
		}
	}()
	_ = MustParse("not-a-version")
}
