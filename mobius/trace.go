package mobius

import (
	"fmt"
	"github.com/mobius-scheduler/mobius/vrp"
)

// extend hull (for tracing entire frontier)
func (s *Mobius) extend_hull_trace(face []fpoint, hull []fpoint) []fpoint {
	s.assert_face_dim(face)

	// find extension
	fp, err := s.find_extension(face, hull)
	if err != nil {
		return face
	} else {
		hull = append(hull, fp)
		var frontier []fpoint
		for idx, _ := range face {
			x := create_candidate_face(fp, face, idx)
			frontier = append(frontier, s.extend_hull_trace(x, hull)...)
		}
		return frontier
	}
}

// remove duplicates from hull
func (s *Mobius) clean_hull(hull []fpoint) []fpoint {
	// find set of allocations
	found := make(map[string]fpoint)
	for _, h := range hull {
		var label string
		for _, id := range s.app_ids {
			label += fmt.Sprintf("%0.1f ", h.weights[id])
		}
		found[label] = h
	}

	// convert to list
	var ch []fpoint
	for _, s := range found {
		ch = append(ch, s)
	}
	return ch
}

// trace convex hull of allocations
func (s *Mobius) TraceFrontier() []vrp.Schedule {
	var hull []fpoint
	hull = s.init_hull()
	hull = s.extend_hull_trace(hull, hull)
	hull = s.clean_hull(hull)
	return extract_schedules(hull)
}
