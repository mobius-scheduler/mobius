package app

import "github.com/mobius-scheduler/mobius/common"

type Application interface {
	Init(AppConfig)
	GetID() int
	GetInterestMap() common.InterestMap
	Update([]common.TaskData, int)
}

type AppConfig struct {
	AppID  int         `json:"app_id"`
	Type   string      `json:"type"`
	Config interface{} `json:"config"`
}

func MergeInterestMaps(ims []common.InterestMap) common.InterestMap {
	im := make(common.InterestMap)
	for _, x := range ims {
		for t, _ := range x {
			im[t] = x[t]
		}
	}
	return im
}
