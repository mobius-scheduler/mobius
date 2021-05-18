package vrp

import (
	"github.com/mobius-scheduler/mobius/common"
	log "github.com/sirupsen/logrus"
	"math"
	"sort"
)

// roi (greedy) solver
type RoiSolver struct {
	solver                  Solver
	interest_map            common.InterestMap
	vehicles                []common.Vehicle
	budget                  int
	unweighted_interest_map common.InterestMap
	Alpha                   float64
}

func (r *RoiSolver) SetInterestMap(im common.InterestMap) {
	r.interest_map = im
}

func (r *RoiSolver) GetInterestMap() common.InterestMap {
	return r.interest_map
}

func (r *RoiSolver) GetRTH() bool { return false }

func (r *RoiSolver) SetInitialSchedule(s Schedule) {}

func (r *RoiSolver) SetTravelTimeMatrixPath(p string) {}
func (r *RoiSolver) GetTravelTimeMatrixPath() string  { return "" }

func (r *RoiSolver) Set(im, uim common.InterestMap, v []common.Vehicle, b, c int, x bool) {
	r.interest_map = im
	r.unweighted_interest_map = uim
	r.vehicles = v
	r.budget = b
}

func (r *RoiSolver) time_left(et []int) []int {
	tl := make([]int, len(et))
	done_count := 0
	for i, _ := range et {
		if r.budget >= et[i] {
			done_count++
		}
		tl[i] = int(math.Max(0, float64(r.budget-et[i])))
	}

	if done_count == len(et) {
		return nil
	} else {
		return tl
	}
}

// initial interest (in order to evaluate utility function)
const EPSILON = 0.1

// compute alpha-utility
func (r *RoiSolver) utility(a Allocation, td *common.TaskData) float64 {
	var u float64
	if r.Alpha == 1 {
		for id, x := range a {
			// incorporate task td
			if td != nil && td.AppID == id {
				x += td.Interest
			}
			if x > 0 {
				u += math.Log(x)
			} else {
				u += math.Log(EPSILON)
			}
		}
	} else {
		for id, x := range a {
			// incorporate task td
			if td != nil && td.AppID == id {
				x += td.Interest
			}
			if x > 0 {
				u += math.Pow(x, 1-r.Alpha) / (1 - r.Alpha)
			} else {
				u += math.Pow(EPSILON, 1-r.Alpha) / (1 - r.Alpha)
			}
		}
	}
	return u
}

func (r *RoiSolver) reweight_alpha(im common.InterestMap, h Allocation) common.InterestMap {
	imw := make(common.InterestMap)
	curr_util := r.utility(h, nil)
	for task, data := range im {
		next_util := r.utility(h, &data)
		if curr_util > next_util {
			log.Fatal("[vrp] curr util > next util in ROI")
		}
		imw[task] = common.TaskData{
			AppID:           data.AppID,
			Location:        data.Location,
			Interest:        next_util - curr_util,
			TaskTimeSeconds: data.TaskTimeSeconds,
		}
	}
	return imw
}

// inputs to instance of ROI problem
type roi_problem struct {
	interest_map common.InterestMap
	vehicles     []common.Vehicle
	budgets      []int
	historical   Allocation
}

// inputs to compute roi ordering
type roi_task struct {
	task        common.Task
	travel_time int
	roi         float64
}

func (r *RoiSolver) sort_by_roi(im common.InterestMap, v common.Vehicle) []roi_task {
	// compute list of tasks with roi
	tasks := make([]roi_task, len(im))
	i := 0
	for task, data := range im {
		tt := travel_time(v.Location, task.Location, v.Speed, data.TaskTimeSeconds)
		roi_val := data.Interest / float64(tt)
		tasks[i] = roi_task{
			task:        task,
			travel_time: tt,
			roi:         roi_val,
		}
		i++
	}

	// sort tasks by roi
	sort.Slice(
		tasks, func(i, j int) bool { return tasks[i].roi > tasks[j].roi },
	)
	return tasks
}

// find best feasible task from sorted list
func find_feasible_task(tasks []roi_task, elapsed, budget int) *roi_task {
	for _, t := range tasks {
		if elapsed+t.travel_time < budget && t.task.AppID == tasks[0].task.AppID {
			return &t
		}
	}
	return nil
}

func midpoint(src, dst common.Location) common.Location {
	// convert to radians
	lat1 := src.Latitude * (math.Pi / 180)
	lon1 := src.Longitude * (math.Pi / 180)
	lat2 := dst.Latitude * (math.Pi / 180)
	lon2 := dst.Longitude * (math.Pi / 180)

	bx := math.Cos(lat2) * math.Cos(lon2-lon1)
	by := math.Cos(lat2) * math.Sin(lon2-lon1)
	lat3 := math.Atan2(
		math.Sin(lat1)+math.Sin(lat2),
		math.Sqrt((math.Cos(lat1)+bx)*(math.Cos(lat1)+bx)+math.Pow(by, 2)),
	)
	lon3 := lon1 + math.Atan2(by, math.Cos(lat1)+bx)

	return common.Location{
		Latitude:  math.Round((lat3 * (180 / math.Pi) * 1e5)) / 1e5,
		Longitude: math.Round((lon3 * (180 / math.Pi) * 1e5)) / 1e5,
	}
}

// inter alpha-fair task in pre-existing schedule
func (r *RoiSolver) insert_alpha_task(
	app_id int,
	p []common.Task,
	st map[common.Task]bool,
	tolerance int,
	v common.Vehicle) (*common.Task, int, int) {
	type insert_task struct {
		task       common.Task
		extra_time int
		idx        int
	}

	var candidates []insert_task
	for i := 1; i < len(p); i++ {
		// extract segment, compute midpoint
		seg := p[i-1 : i+1]
		mp := midpoint(seg[0].Location, seg[1].Location)

		// find insertable tasks
		for task, data := range r.interest_map {
			_, sched := st[task]
			if task.AppID == app_id && !sched {
				tt := travel_time(
					mp,
					task.Location,
					v.Speed,
					data.TaskTimeSeconds,
				)
				if tt < tolerance {
					candidates = append(
						candidates,
						insert_task{
							task:       task,
							extra_time: tt,
							idx:        i,
						},
					)
				}
			}
		}
	}

	// sort by least extra time
	sort.Slice(
		candidates,
		func(i, j int) bool {
			return candidates[i].extra_time < candidates[j].extra_time
		},
	)

	if len(candidates) == 0 {
		return nil, 0, 0
	}
	c := candidates[0]
	return &c.task, c.extra_time, c.idx
}

// compute set of alpha-fair tasks
func (r *RoiSolver) compute_alpha_tasks(p roi_problem) map[common.Task]bool {
	sched_tasks := make(map[common.Task]bool)
	for i, v := range p.vehicles {
		elapsed := 0

		// complete as many tasks as reachable
		var path []common.Task
		for elapsed < p.budgets[i] && len(p.interest_map) > 0 {
			// reweight im, get tasks sorted by roi
			imw := r.reweight_alpha(p.interest_map, p.historical)
			tasks_sorted := r.sort_by_roi(imw, v)

			// find best (feasible) roi task
			next := find_feasible_task(tasks_sorted, elapsed, p.budgets[i])

			// if no feasible task, try to insert
			if next == nil {
				tol := math.Min(
					float64(tasks_sorted[0].travel_time/2),
					float64(p.budgets[i]-elapsed),
				)
				task, et, pos := r.insert_alpha_task(
					tasks_sorted[0].task.AppID,
					path,
					sched_tasks,
					int(tol),
					v,
				)

				if task == nil {
					break
				}

				// insert in path
				path = append(path, common.Task{})
				copy(path[pos+1:], path[pos:])
				path[pos] = *task
				sched_tasks[*task] = true
				elapsed += 2 * et
				p.historical[task.AppID] += p.interest_map[*task].Interest
			} else {
				path = append(path, next.task)
				sched_tasks[next.task] = true
				elapsed += next.travel_time
				p.historical[next.task.AppID] += p.interest_map[next.task].Interest
				v.Location = next.task.Location
				delete(p.interest_map, next.task)
			}
		}
	}
	return sched_tasks
}

// reorder alpha-fair tasks with VRP
func (r *RoiSolver) reorder_with_vrp(im common.InterestMap, budget int) Schedule {
	solver := NewGoogleSolver(im, im, r.vehicles, budget, 0, nil)
	return solver.Solve()
}

// perform final packing, with fair set of tasks
func (r *RoiSolver) final_pack(ft map[common.Task]bool, fs Schedule) Schedule {
	// create IM with bias on fair tasks
	im := make(common.InterestMap)
	for task, _ := range r.interest_map {
		im[task] = r.interest_map[task]
		if _, ok := ft[task]; ok {
			var data = im[task]
			data.Interest = 1000.0
			im[task] = data
		}
	}

	// generate packed schedule
	solver := NewGoogleSolver(im, r.interest_map, r.vehicles, r.budget, 0, nil)
	solver.SetInitialSchedule(fs)
	return solver.Solve()
}

// compute schedule with ROI
func (r *RoiSolver) Solve() Schedule {
	time_left := make([]int, len(r.vehicles))
	for i, _ := range time_left {
		time_left[i] = r.budget
	}

	// create copy of im
	im := make(common.InterestMap)
	for t, d := range r.interest_map {
		im[t] = d
	}

	// create copy of vehicles
	v := make([]common.Vehicle, len(r.vehicles))
	for i, x := range r.vehicles {
		v[i] = x
	}

	// init historical
	historical := make(Allocation)
	app_ids := r.interest_map.GetApps()
	for _, id := range app_ids {
		historical[id] = 0
	}

	var sched Schedule
	var fair_tasks map[common.Task]bool
	for time_left != nil {

		// compute alpha tasks
		fair_tasks = r.compute_alpha_tasks(
			roi_problem{
				interest_map: im,
				vehicles:     v,
				budgets:      time_left,
				historical:   historical,
			},
		)

		// create interestmap of fair tasks and generate schedule
		imf := make(common.InterestMap)
		for task, _ := range fair_tasks {
			imf[task] = r.interest_map[task]
		}
		sched = r.reorder_with_vrp(imf, r.budget)
		time_left = r.time_left(sched.ElapsedTime())
	}

	// pack schedule with additional tasks
	return r.final_pack(fair_tasks, sched)
}
