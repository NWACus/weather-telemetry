package parse

import "testing"

func TestParseName(t *testing.T) {
	for _, testCase := range []struct {
		path        string
		name        string
		year        int
		shouldError bool
	}{
		{
			path: "ALL0708.DAT",
			name: "ALL",
			year: 2007,
		},
		{
			path: "15GUN2526.cr1",
			name: "GUN",
			year: 2025,
		},
		{
			path: "ESS0708 (1).CR1",
			name: "ESS",
			year: 2007,
		},
		{
			path: "CML1718_withPrecip.dat",
			name: "CML",
			year: 2017,
		},
	} {
		t.Run(testCase.path, func(t *testing.T) {
			name, year, err := parseName(testCase.path)
			if (err != nil) != testCase.shouldError {
				t.Errorf("expected error %v, got %v", testCase.shouldError, err)
			}
			if testCase.shouldError {
				return
			}
			if name != testCase.name {
				t.Errorf("expected name %s, got %s", testCase.name, name)
			}
			if year != testCase.year {
				t.Errorf("expected year %d, got %d", testCase.year, year)
			}
		})
	}
}
