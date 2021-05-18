package mobius

import (
	"errors"
	"github.com/mobius-scheduler/mobius/common"
	"github.com/mobius-scheduler/mobius/vrp"
	log "github.com/sirupsen/logrus"
	"math"
	"sort"
)

// compute lagrangian
func (s *Mobius) compute_lagrangian(w map[int]float64, c float64) float64 {
	var d float64
	for _, weight := range w {
		d += math.Pow(weight, 1-1/s.Alpha)
	}
	return math.Pow(c/d, -s.Alpha)
}

// compute value of optimal allocation
func (s *Mobius) compute_opt(face []fpoint) ([]float64, error) {
	c, weights, err := s.compute_face_equation(face)
	if err != nil {
		return nil, errors.New("error computing face equation")
	}
	w := s.weight_vector_to_map(weights)
	lambda := s.compute_lagrangian(w, c)

	// compute opt
	x_opt := make([]float64, s.num_apps)
	for i, id := range s.app_ids {
		x_opt[i] = math.Pow(lambda*w[id], -1/s.Alpha)
	}
	return x_opt, nil
}

// determine if opt allocation lies within face
func (s *Mobius) opt_in_face(opt []float64, app_allocs map[int][]float64) bool {
	for i, id := range s.app_ids {
		min, max := common.GetMinMax(app_allocs[id])
		valid := opt[i] >= min && opt[i] <= max
		if !valid {
			return false
		}
	}
	return true
}

// check if utility-maximizing solution lies in face
func (s *Mobius) eval_face(face []fpoint) bool {
	x_opt, err := s.compute_opt(face)
	if err != nil {
		return false
	}

	// organize by allocs to each app
	// map app_id --> list of allocs
	app_allocs := make(map[int][]float64)
	for i, f := range face {
		for _, id := range s.app_ids {
			if _, ok := app_allocs[id]; !ok {
				app_allocs[id] = make([]float64, len(face))
			}
			app_allocs[id][i] = f.schedule.Allocation[id]
		}
	}
	return s.opt_in_face(x_opt, app_allocs)
}

// extend hull (in direction of alpha-fair solution)
func (s *Mobius) extend_hull_search(face []fpoint, hull []fpoint) []fpoint {
	s.assert_face_dim(face)

	var alloc []vrp.Allocation
	for _, f := range face {
		alloc = append(alloc, f.schedule.Allocation)
	}

	// find extension
	fp, err := s.find_extension(face, hull)
	if err != nil {
		log.Warnf("error %v", err)
		return face
	} else {
		hull = append(hull, fp)

		for idx, _ := range face {
			x := create_candidate_face(fp, face, idx)
			if s.eval_face(x) {
				log.Debugln("**** considering face ****")
				for _, a := range x {
					log.Debugf("alloc %v, util %v", a.schedule.Allocation, a.utility)
				}
				log.Debugln("**** end face ****")
				return s.extend_hull_search(x, hull)
			}
		}
		log.Debugln("no intersecting face found")
		return append(face, fp)
	}
}

// search for most alpha-fair allocation on convex hull
func (s *Mobius) SearchFrontier() vrp.Schedule {

	// compute face if needed
	if s.last_face == nil {
		hull := s.init_hull()
		s.last_face = s.extend_hull_search(hull, hull)

		// verify that we end on a face
		s.assert_face_dim(s.last_face)
	} else {
		for idx, _ := range s.last_face {
			s.last_face[idx].utility = s.utility(s.last_face[idx].schedule.Allocation)
		}
	}

	// choose best solution on face
	// (1) max utility, (2) max total interest
	sort.Slice(
		s.last_face,
		func(i, j int) bool {
			if s.last_face[i].utility > s.last_face[j].utility {
				return true
			}
			if s.last_face[i].utility < s.last_face[j].utility {
				return false
			}
			return s.last_face[i].schedule.Allocation.Total() > s.last_face[j].schedule.Allocation.Total()
		},
	)

	log.Debugln("**** sorted hull ****")
	for _, h := range s.last_face {
		log.Debugf(
			"%v, total %v, utility %v",
			h.schedule.Allocation,
			h.schedule.Allocation.Total(),
			h.utility,
		)
	}
	log.Debugln("**** end hull ****")

	return s.last_face[0].schedule
}
