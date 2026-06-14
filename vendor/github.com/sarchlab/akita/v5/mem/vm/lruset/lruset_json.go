package lruset

import "encoding/json"

// setJSON is the JSON form of a Set. The Set's fields are unexported, so without
// these methods encoding/json would serialize a Set as an empty object and
// silently drop the visit order and key map — leaving a restored TLB/mmuCache
// set unable to evict (its visit list would be empty).
type setJSON struct {
	WayCount   int            `json:"way_count"`
	VisitList  []int          `json:"visit_list"`
	VisitCount uint64         `json:"visit_count"`
	LastVisits []uint64       `json:"last_visits"`
	KeyMap     map[string]int `json:"key_map"`
}

// MarshalJSON serializes the full LRU state. It is a value receiver so the Set
// round-trips even when held as a value field (e.g. setState.LRU).
func (s Set) MarshalJSON() ([]byte, error) {
	return json.Marshal(setJSON{
		WayCount:   s.wayCount,
		VisitList:  s.visitList,
		VisitCount: s.visitCount,
		LastVisits: s.lastVisits,
		KeyMap:     s.keyMap,
	})
}

// UnmarshalJSON restores the full LRU state. keyMap is always left non-nil so
// later UpdateKey/Remove calls do not write to a nil map.
func (s *Set) UnmarshalJSON(data []byte) error {
	var dto setJSON
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}

	s.wayCount = dto.WayCount
	s.visitList = dto.VisitList
	s.visitCount = dto.VisitCount
	s.lastVisits = dto.LastVisits
	s.keyMap = dto.KeyMap
	if s.keyMap == nil {
		s.keyMap = make(map[string]int)
	}

	return nil
}
