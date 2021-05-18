package mobius

import (
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/mobius-scheduler/mobius/common"
	"github.com/mobius-scheduler/mobius/vrp"
	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/mat"
	"math"
	"sort"
	"sync"
)

type Mobius struct {
	InterestMap     common.InterestMap
	Solver          vrp.Solver
	Vehicles        []common.Vehicle
	Horizon         int
	Capacity        int
	Historical      vrp.Allocation
	Dir             string
	Alpha           float64
	Discount        float64
	app_ids         []int
	num_apps        int
	min_app_id      int
	heuristics      map[string]vrp.Schedule
	last_face       []fpoint
	frontier_writer *csv.Writer
}

// initial interest (in order to evaluate utility function)
const EPSILON = 0.1

// init mobius, compute warm start schedules
func (s *Mobius) Init() {
	s.app_ids = s.InterestMap.GetApps()
	s.num_apps = len(s.app_ids)
	if s.num_apps < 1 {
		log.Fatalf("[mobius] found %d apps; must have at least 1", s.num_apps)
	}
	s.min_app_id = s.app_ids[0]

	// setup logging: CSV of allocations
	if s.Dir != "" {
		s.frontier_writer = common.CreateCSVWriter(s.Dir + "/frontier.csv")

		// write header
		// solver, app1, ..., appN
		header := make([]string, 2+s.num_apps)
		header[0] = "env"
		header[1] = "solver"
		for i := 2; i < len(header); i++ {
			header[i] = fmt.Sprintf("app%d", i-1)
		}
		s.frontier_writer.Write(header)
	}

	// compute warm start schedules
	s.heuristics = make(map[string]vrp.Schedule)
	s.warm_start()
	s.last_face = nil
}

func (s *Mobius) get_csv_row(solver string, alloc vrp.Allocation) []string {
	row := make([]string, 2+s.num_apps)
	row[0] = s.Dir
	row[1] = solver
	for _, id := range s.app_ids {
		row[id+1] = fmt.Sprintf("%0.2f", alloc[id])
	}
	return row
}

// precompute schedules to bootstrap solver
// we parallelize the computation
func (s *Mobius) warm_start() {
	if s.frontier_writer != nil {
		defer s.frontier_writer.Flush()
	}

	type ws struct {
		schedule vrp.Schedule
		label    string
	}
	alphas := []float64{0.1, 0.25, 1.0, 5.0, 100.0}
	var c chan ws
	if s.Solver.GetRTH() != nil {
		c = make(chan ws, 2)
	} else {
		c = make(chan ws, 2+len(alphas))
	}
	var wg sync.WaitGroup

	// dedicate vehicle per app
	if len(s.Vehicles)%s.num_apps == 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := vrp.DedicateSolver{}
			d.Set(
				s.InterestMap,
				s.InterestMap,
				s.Vehicles,
				s.Horizon,
				s.Capacity,
				s.Solver.GetRTH(),
			)
			d.SetTravelTimeMatrixPath(s.Solver.GetTravelTimeMatrixPath())
			sched := d.Solve()
			log.Debugf(
				"warm start: dedicate: %v, util %v",
				sched.Allocation,
				s.utility(sched.Allocation),
			)
			c <- ws{schedule: sched, label: "dedicate"}
		}()
	}

	// max throughput schedule (standard VRP)
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.Solver.Set(
			s.InterestMap,
			s.InterestMap,
			s.Vehicles,
			s.Horizon,
			s.Capacity,
			s.Solver.GetRTH(),
		)
		sched := s.Solver.Solve()
		log.Debugf(
			"warm start: maxthp: %v, util %v",
			sched.Allocation,
			s.utility(sched.Allocation),
		)
		c <- ws{schedule: sched, label: "maxthp"}
	}()

	// roi for different alphas
	// only works when no RTH
	if s.Solver.GetRTH() == nil && s.Solver.GetTravelTimeMatrixPath() == "" {
		for _, a := range alphas {
			wg.Add(1)
			go func(alpha float64) {
				defer wg.Done()
				solver := vrp.RoiSolver{Alpha: alpha}
				solver.Set(s.InterestMap, s.InterestMap, s.Vehicles, s.Horizon, 0, false)
				sched := solver.Solve()
				log.Debugf(
					"warm start: roi, alpha %v: %v, util %v",
					alpha,
					sched.Allocation,
					s.utility(sched.Allocation),
				)
				c <- ws{schedule: sched, label: fmt.Sprintf("roi_alpha%v", alpha)}
			}(a)
		}
	}

	// wait for threads to finish
	wg.Wait()
	close(c)
	for x := range c {
		s.heuristics[x.label] = x.schedule
		if s.frontier_writer != nil {
			s.frontier_writer.Write(
				s.get_csv_row(x.label, x.schedule.Allocation),
			)
		}
	}
}

// compute alpha-utlitity of allocation
func (s *Mobius) utility(a vrp.Allocation) float64 {
	// incorporate historical interest
	h := make(vrp.Allocation)
	for _, id := range s.app_ids {
		h[id] = s.Discount*s.Historical[id] + a[id]
	}

	var u float64
	if s.Alpha == 1 {
		for _, x := range h {
			if x > 0 {
				u += math.Log(x)
			} else {
				u += math.Log(EPSILON)
			}
		}
	} else {
		for _, x := range h {
			if x > 0 {
				u += math.Pow(x, 1-s.Alpha) / (1 - s.Alpha)
			} else {
				u += math.Pow(EPSILON, 1-s.Alpha) / (1 - s.Alpha)
			}
		}
	}
	return u
}

// compute weighted reward, according to weight vector applied on InterestMap
func weighted_reward(w map[int]float64, allocation vrp.Allocation) float64 {
	var reward float64
	for id, _ := range allocation {
		reward += w[id] * allocation[id]
	}
	return reward
}

// thread safe
func compute_schedule_ts(w map[int]float64, solver vrp.Solver) vrp.Schedule {
	schedule := solver.Solve()
	schedule.Stats.Weights = w
	return schedule
}

// reweight interestmap and run VRP
func (s *Mobius) compute_schedule(w map[int]float64) (vrp.Schedule, float64) {

	// check that num weights == num apps
	if len(w) != s.num_apps {
		log.Fatalf(
			"[mobius] cannot reweight InterestMap: %d weights, %d apps",
			len(w),
			s.num_apps,
		)
	}

	solver := s.Solver.New()

	imw := s.InterestMap.Reweight(w)
	initial_schedule := s.choose_init_schedule(w)
	solver.Set(imw, s.InterestMap, s.Vehicles, s.Horizon, s.Capacity, s.Solver.GetRTH())
	solver.SetInitialSchedule(initial_schedule)
	schedule := solver.Solve()

	// assert that schedule improved
	ok := assert_schedule_improved(w, schedule.Allocation, initial_schedule.Allocation)
	if !ok {
		log.Warnf(
			"[mobius] schedule did not improve for weight %v: %v --> %v",
			w,
			initial_schedule.Allocation,
			schedule.Allocation,
		)
	}
	log.Debugf(
		"schedule for weights %v = %v, util %v",
		w,
		schedule.Allocation,
		s.utility(schedule.Allocation),
	)

	// write allocation to log
	if s.frontier_writer != nil {
		defer s.frontier_writer.Flush()
		s.frontier_writer.Write(
			s.get_csv_row(
				"vrp",
				schedule.Allocation,
			),
		)
	}

	schedule.Stats.Weights = w
	schedule.Stats.Alpha = s.Alpha

	return schedule, s.utility(schedule.Allocation)
}

// compute best (highest weighted reward) schedule
// from `heuristics` (bank of cached schedules)
func (s *Mobius) choose_init_schedule(w map[int]float64) vrp.Schedule {
	type ws_schedule struct {
		schedule        vrp.Schedule
		weighted_reward float64
	}

	// compute weighted reward for each schedule
	schedules := make([]ws_schedule, len(s.heuristics)+1)
	var idx int
	for _, h := range s.heuristics {
		schedules[idx] = ws_schedule{
			schedule:        h,
			weighted_reward: weighted_reward(w, h.Allocation),
		}
		idx++
	}

	// sort and choose best
	sort.Slice(
		schedules,
		func(i, j int) bool {
			return schedules[i].weighted_reward > schedules[j].weighted_reward
		},
	)
	log.Debugf("choosing init schedule: %v", schedules[0].schedule.Allocation)
	return schedules[0].schedule
}

// assert that weighted reward of schedule is higher than
// that of initial schedule
func assert_schedule_improved(w map[int]float64, final, init vrp.Allocation) bool {
	return weighted_reward(w, final) >= weighted_reward(w, init)
}

// schema for point on the frontier
type fpoint struct {
	schedule vrp.Schedule
	utility  float64
	weights  map[int]float64
}

// check if current frontier contains allocation
func contains(hull []fpoint, allocation vrp.Allocation) bool {
	for _, p := range hull {
		match := true
		for id, a := range p.schedule.Allocation {
			if allocation[id] != a {
				match = false
			}
		}
		if match {
			return true
		}
	}
	return false
}

// extract schedules from hull
func extract_schedules(hull []fpoint) []vrp.Schedule {
	schedules := make([]vrp.Schedule, len(hull))
	for i, h := range hull {
		schedules[i] = h.schedule
	}
	return schedules
}

// init hull with single-app schedules
// we parallelize, since each schedule is independent
func (s *Mobius) init_hull() []fpoint {
	var hull []fpoint
	c := make(chan fpoint, len(s.app_ids))
	var wg sync.WaitGroup

	for _, id := range s.app_ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			weights := make(map[int]float64)
			for _, idx := range s.app_ids {
				if idx == i {
					weights[idx] = 1.0
				} else {
					weights[idx] = 0.0
				}
			}
			schedule, _ := s.compute_schedule(weights)
			// assert that initialization is useful
			if schedule.Allocation[i] == 0 {
				log.Fatalf("app %v, nothing allocated", i)
			}
			c <- fpoint{
				schedule: schedule,
				utility:  s.utility(schedule.Allocation),
				weights:  weights,
			}
		}(id)
	}

	// wait for threads to finish
	wg.Wait()
	close(c)
	for x := range c {
		hull = append(hull, x)
	}

	return hull
}

// compute equation of face
// A face is determined by num_app allocations.
// We solve a system of equations using these known allocations
// to compute the normal vector to this face.
func (s *Mobius) compute_face_equation(face []fpoint) (float64, []float64, error) {
	// make sure system isn't underconstrained in app1 dimension
	var uc bool
	for _, f := range face {
		if f.schedule.Allocation[s.min_app_id] > 0 {
			uc = true
		}
	}
	if !uc {
		return 0.0, nil, errors.New("underconstrained in app1")
	}

	// create constraint matrix: len(face) x num_apps
	// create b (app1) vector: len(face)
	A := mat.NewDense(len(face), s.num_apps, nil)
	b := mat.NewVecDense(len(face), nil)
	for fid, f := range face {
		row := []float64{1}
		for _, id := range s.app_ids {
			if id != s.min_app_id {
				row = append(row, -f.schedule.Allocation[id])
			}
		}
		A.SetRow(fid, row)
		b.SetVec(fid, f.schedule.Allocation[s.min_app_id])
	}

	// solve system of equations
	// x = A\b
	x := mat.NewVecDense(s.num_apps, nil)
	if err := x.SolveVec(A, b); err != nil {
		log.Warnf("[mobius] could not compute face equation: %v", err)
		for _, f := range face {
			log.Warnf("weights %v, allocation %v", f.weights, f.schedule.Allocation)
		}
		return 0.0, nil, errors.New("underconstrained")
	}

	// convert to slice
	weights := mat.Col(nil, 0, x)

	return weights[0], append([]float64{1}, weights[1:len(weights)]...), nil
}

// check if weight vector is valid (i.e., all elements are nonzero)
func valid_weights(w []float64) bool {
	for _, x := range w {
		if x < 0 {
			return false
		}
	}
	return true
}

// convert weight vector (slice) -> map (by app id)
func (s *Mobius) weight_vector_to_map(weights []float64) map[int]float64 {
	w := make(map[int]float64)
	for idx, id := range s.app_ids {
		w[id] = weights[idx]
	}
	return w
}

// create tag from app weights, in order to index corresponding schedule
func (s *Mobius) weight_tag(w map[int]float64) string {
	var tag string
	for _, id := range s.app_ids {
		tag += fmt.Sprintf("%0.2f_", w[id])
	}
	return tag
}

// find feasible extension to convex hull
func (s *Mobius) find_extension(face []fpoint, hull []fpoint) (fpoint, error) {
	// compute face equation
	c, weights, err := s.compute_face_equation(face)
	if err != nil {
		return fpoint{}, errors.New(fmt.Sprintf("no extension found: %v", err))
	}
	if !valid_weights(weights) {
		return fpoint{}, errors.New(fmt.Sprintf("no extension found (invalid weights): %v", err))
	}
	w := s.weight_vector_to_map(weights)

	// reweight InterestMap and compute schedule
	schedule, utility := s.compute_schedule(w)

	wr := weighted_reward(w, schedule.Allocation)
	if wr >= c && !contains(hull, schedule.Allocation) {
		// add schedule to heuristic bank
		s.heuristics["weight_"+s.weight_tag(w)] = schedule

		fp := fpoint{
			schedule: schedule,
			utility:  utility,
			weights:  w,
		}
		return fp, nil
	} else {
		return fpoint{}, errors.New("no extension found: no better schedule")
	}
}

// check that dimension of face is correct
func (s *Mobius) assert_face_dim(face []fpoint) {
	if len(face) < s.num_apps {
		log.Fatalf(
			"[mobius] face has dimension %d, expected %d-%d",
			len(face),
			s.num_apps,
			s.num_apps+1,
		)
	}
}

// create candidate face from new point and current face
func create_candidate_face(fp fpoint, face []fpoint, exclude_idx int) []fpoint {
	x := make([]fpoint, len(face))
	x[0] = fp
	n := 1
	for i, _ := range face {
		if i != exclude_idx {
			x[n] = face[i]
			n++
		}
	}
	return x
}
