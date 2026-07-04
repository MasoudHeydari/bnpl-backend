package invariants

import "strings"

// EventTiming renders PostgreSQL's stable tgtype bits for developer-facing tools.
func (in Invariant) EventTiming() string {
	timing := "AFTER"
	if in.TriggerType&2 != 0 {
		timing = "BEFORE"
	}
	var events []string
	for _, e := range []struct {
		bit  int16
		name string
	}{{4, "INSERT"}, {16, "UPDATE"}, {8, "DELETE"}, {32, "TRUNCATE"}} {
		if in.TriggerType&e.bit != 0 {
			events = append(events, e.name)
		}
	}
	return timing + " " + strings.Join(events, " OR ")
}
