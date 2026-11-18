package jalali

import (
	"testing"
	"time"
)

func TestStartOfFollowingTehranMonth(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		from Date
		want Date
	}{
		{"first of month", Date{Year: 1405, Month: 8, Day: 1}, Date{Year: 1405, Month: 9, Day: 1}},
		{"end of month", Date{Year: 1405, Month: 8, Day: 30}, Date{Year: 1405, Month: 9, Day: 1}},
		{"new year", Date{Year: 1405, Month: 12, Day: 29}, Date{Year: 1406, Month: 1, Day: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := tc.from.ToTime(time.Date(2000, time.January, 1, 17, 0, 0, 0, loc), loc)
			next, err := StartOfFollowingTehranMonth(start)
			if err != nil {
				t.Fatal(err)
			}

			if got := FromTime(next.In(loc)); got != tc.want {
				t.Fatalf("next=%+v want=%+v", got, tc.want)
			}

			if next.In(loc).Hour() != 0 || next.In(loc).Minute() != 0 {
				t.Fatalf("cutoff not midnight: %v", next)
			}
		})
	}
}
