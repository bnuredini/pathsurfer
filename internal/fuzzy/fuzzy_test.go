package fuzzy

import (
	"reflect"
	"testing"
)

func TestGeneral(t *testing.T) {
	data := []struct {
		Pattern    string
		Candidates []string
		Want       []Match
	}{
		{
			"abc",
			[]string{"alpha-beta-cents"},
			[]Match{
				Match{
					CandidateString: "alpha-beta-cents",
					Indexes:         []int{0, 6, 11},
					Score:           19,
				},
			},
		},
		{
			"ag",
			[]string{"thing-angle-thing"},
			[]Match{
				Match{
					CandidateString: "thing-angle-thing",
					Indexes:         []int{6, 8},
					Score:           11,
				},
			},
		},
		{
			"ns",
			[]string{"ns", "clones"},
			[]Match{
				Match{
					CandidateString: "ns",
					Indexes: []int{0, 1},
					Score: 7,
				},
				Match{
					CandidateString: "clones",
					Indexes: []int{3, 5},
					Score: 1,
				},
			},
		},
		{
			"packages",
			[]string{"PackageManagerSettings.asset", "Packages"},
			[]Match{
				Match{
					CandidateString: "Packages",
					Indexes: []int{0, 1, 2, 3, 4, 5, 6, 7},
					Score: 13,
				},
				Match{
					CandidateString: "PackageManagerSettings.asset",
					Indexes: []int{0, 1, 2, 3, 4, 5, 6, 14},
					Score: 12,
				},
			},
		},
		
		// Should we deprioritize separators?
		/*
		{
			"abc",
			[]string{"abc", "another-birthday-cake"},
			[]Match{
				Match{
					CandidateString: "abc",
					Indexes: []int{0, 1, 2},
					Score: 13,
				},
				Match{
					CandidateString: "PackageManagerSettings.asset",
					Indexes: []int{0, 8, 17},
					Score: 12,
				},
			},
		},
		*/
	}

	for _, tt := range data {
		if got, want := Find(tt.Pattern, tt.Candidates), tt.Want; !reflect.DeepEqual(got, want) {
			t.Errorf("want=%+v, got=%+v", want, got)
		}
	}
}
