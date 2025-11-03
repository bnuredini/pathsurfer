package fuzzy

import (
	"reflect"
	"testing"
)

func TestGeneral(t *testing.T) {
	data := []struct{
		Pattern string
		Candidates []string
		Want []Match
	}{
		{
			"abc",
			[]string{"alpha-beta-cents"},
			[]Match{
				Match{
					CandidateString: "alpha-beta-cents",
					Indexes: []int{0, 6, 11},
					Score: 17,
				},
			},
		},
	}

	for _, tt := range data {
		if got, want := Find(tt.Pattern, tt.Candidates), tt.Want; !reflect.DeepEqual(got, want) {
			t.Errorf("want=%+v, got=%+v", want, got)
		}
	}
}
