package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
)

type minimapEntry struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Count     float64 `json:"count"`
}

var minimap []*minimapEntry

func parseMinimap() {
	starts := make([]float64, 0)
	ends := make([]float64, 0)

	for _, inst := range trace {
		starts = append(starts, inst.Events[0].Time)
		ends = append(ends, inst.Events[len(inst.Events)-1].Time)
	}

	sort.Float64s(starts)
	sort.Float64s(ends)

	ptrStarts := 0
	ptrEnds := 0
	count := float64(0)
	now := math.Min(starts[0], ends[0])
	for ptrStarts < len(starts) || ptrEnds < len(ends) {
		var nextTime float64
		isStart := false

		if ptrEnds >= len(ends) {
			nextTime = starts[ptrStarts]
			ptrStarts++
			isStart = true
		} else if ptrStarts >= len(starts) {
			nextTime = ends[ptrEnds]
			ptrEnds++
			isStart = false
		} else if starts[ptrStarts] < ends[ptrEnds] {
			nextTime = starts[ptrStarts]
			ptrStarts++
			isStart = true
		} else {
			nextTime = ends[ptrEnds]
			ptrEnds++
			isStart = false
		}

		if nextTime != now {
			entry := &minimapEntry{now, nextTime, count}
			if entry.StartTime >= entry.EndTime {
				log.Fatal(entry)
			}

			minimap = append(minimap, entry)
			now = nextTime
		}

		if isStart {
			count++
		} else {
			count--
		}

	}
}

func resample(numSample int) []*minimapEntry {
	resampledMinimap := make([]*minimapEntry, 0)
	startTime := minimap[0].StartTime
	endTime := minimap[len(minimap)-1].EndTime
	stepSize := (endTime - startTime) / float64(numSample)

	instCount := float64(0)
	binCount := 0
	now := startTime
	for i := 0; i < len(minimap); i++ {
		if minimap[i].EndTime > now+stepSize {
			count := float64(0)
			if binCount != 0 {
				count = float64(instCount) / float64(binCount)
			}
			entry := &minimapEntry{now, now + stepSize, count}
			resampledMinimap = append(resampledMinimap, entry)
			instCount = 0
			binCount = 0
			now = now + stepSize
		}

		instCount += minimap[i].Count
		binCount++
	}

	return resampledMinimap
}

func httpMinimap(w http.ResponseWriter, r *http.Request) {
	numSamples, err := strconv.Atoi(r.FormValue("num_samples"))
	dieOnErr(err)

	var bytes []byte
	if numSamples > 0 && numSamples < len(minimap) {
		minimapResample := resample(numSamples)
		bytes, err = json.Marshal(minimapResample)
		dieOnErr(err)
	} else {
		bytes, err = json.Marshal(minimap)
		dieOnErr(err)
	}

	_, err = w.Write(bytes)
	dieOnErr(err)

}
