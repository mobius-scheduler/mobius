package common

import "sort"

// task data in each entry of interestmap
type TaskData struct {
	AppID           int      `json:"app_id"`
	Location        Location `json:"location"`
	Destination     Location `json:"destination"`
	Interest        float64  `json:"interest"`
	TaskTimeSeconds float64  `json:"task_time_seconds"`
	RequestTime     int      `json:"request_time"`
	FulfillTime     int      `json:"fulfill_time"`
}

// extract task from TaskData
func (t *TaskData) GetTask() Task {
	return Task{
		Location:    t.Location,
		Destination: t.Destination,
		AppID:       t.AppID,
		RequestTime: t.RequestTime,
	}
}

// interestmap object
type InterestMap map[Task]TaskData
type InterestFile []TaskData

// convert InterestMap to InterestFile, where key is string
func (im InterestMap) ToFile() InterestFile {
	interest_file := make(InterestFile, len(im))
	idx := 0
	for _, data := range im {
		interest_file[idx] = data
		idx++
	}
	return interest_file
}

// get set of apps in InterestMap
func (im InterestMap) GetApps() []int {
	apps := make(map[int]struct{})
	for t, _ := range im {
		apps[t.AppID] = struct{}{}
	}

	var app_ids []int
	for k, _ := range apps {
		app_ids = append(app_ids, k)
	}
	sort.Ints(app_ids)
	return app_ids
}

func (im InterestMap) Copy() InterestMap {
	x := make(InterestMap)
	for k, v := range im {
		x[k] = v
	}
	return x
}

// reweight InterestMap according to weights per-app
func (im InterestMap) Reweight(w map[int]float64) InterestMap {
	imw := make(InterestMap)
	for t, _ := range im {
		imw[t] = im[t]
		var task_data = imw[t]
		task_data.Interest = w[t.AppID] * im[t].Interest
		imw[t] = task_data
	}
	return imw
}

// filter InterestMap by app ID
func (im InterestMap) FilterByApp(id int) InterestMap {
	ima := make(InterestMap)
	for t, _ := range im {
		if t.AppID == id {
			ima[t] = im[t]
		}
	}
	return ima
}

// get tasks (keys) in InterestMap
func (im InterestMap) GetTasks() []Task {
	tasks := make([]Task, len(im))
	i := 0
	for t, _ := range im {
		tasks[i] = t
		i++
	}
	return tasks
}

// get total interest in InterestMap
func (im InterestMap) GetTotalInterest() float64 {
	var total float64
	for t, _ := range im {
		total += im[t].Interest
	}
	return total
}
