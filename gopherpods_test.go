package gopherpods

import "testing"

func Test_parseOrder(t *testing.T) {
	var tt = []struct {
		in   string
		want string
	}{
		{"", "-Date"},
		{"date", "-Date"},
		{"title", "Title"},
		{"show", "Show"},
	}

	for _, test := range tt {
		got := parseOrder(test.in)

		if got != test.want {
			t.Errorf("parseOrder(%s) = %s, want %s", test.in, got, test.want)
		}
	}
}
