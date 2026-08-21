package ranking

import "sort"

type Status string

const (
	StatusDNF Status = "DNF"
	StatusDSQ Status = "DSQ"
	StatusDNS Status = "DNS"
)

type TeamResult struct {
	TeamID      string
	TimeSeconds *float64
	Status      Status
}

func statusOrder(s Status) int {
	switch s {
	case StatusDNF:
		return 1
	case StatusDSQ:
		return 2
	case StatusDNS:
		return 3
	default:
		return 0
	}
}

// Rank returns a new slice sorted fastest-first: teams with a recorded
// time sort ascending by that time; teams with a status instead of a
// time sort after every timed team, in the order DNF, DSQ, DNS. Ties
// keep their relative input order. The input slice is not modified.
func Rank(results []TeamResult) []TeamResult {
	ranked := make([]TeamResult, len(results))
	copy(ranked, results)

	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		aHasTime := a.TimeSeconds != nil
		bHasTime := b.TimeSeconds != nil

		if aHasTime && bHasTime {
			return *a.TimeSeconds < *b.TimeSeconds
		}
		if aHasTime != bHasTime {
			return aHasTime
		}
		return statusOrder(a.Status) < statusOrder(b.Status)
	})

	return ranked
}
