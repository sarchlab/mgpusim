package resource

// allocStatus represents the allocation status of SGPRs, VGPRs, or LDS units
type allocStatus byte

// A list of possible status for CU binded storage allocation
const (
	allocStatusFree      allocStatus = iota
	allocStatusToReserve             // A value that is used for reservation caculation
	allocStatusReserved              // Work-Group mapped, but wavefront not dispatched
	allocStatusUsed                  // Currently in use
)

// Mask is checkpointed allocation data, including the cursor for unlimited resources.
type Mask struct {
	Unlimited bool
	Next      int
	Bits      []allocStatus
}

func newResourceMask(size int) Mask { return Mask{Bits: make([]allocStatus, size)} }

// nextRegion finds a region that is masked by the resourceMask in
// the state define by statusReq. This function returns the offset of the
// starting point of the region. It also returns a boolean value that
// tells if a region is found
func (m *Mask) nextRegion(
	length int,
	statusReq allocStatus,
) (int, bool) {
	if m.Unlimited {
		point := m.Next
		m.Next += length
		return point, true
	}
	if length == 0 {
		return 0, true
	}

	offset := 0
	currLength := 0
	for offset < len(m.Bits) {
		if m.Bits[offset] == statusReq {
			currLength++
			if currLength == length {
				return offset - currLength + 1, true
			}
		} else {
			currLength = 0
		}
		offset++
	}
	return 0, false
}

// setStatus alters the status from the position of offset to offset + length
func (m *Mask) setStatus(offset, length int, status allocStatus) {
	if m.Unlimited {
		return
	}
	for i := 0; i < length; i++ {
		m.Bits[offset+i] = status
	}
}

// convertStatus change all the element of one status to another
func (m *Mask) convertStatus(from, to allocStatus) {
	if m.Unlimited {
		return
	}
	for i := 0; i < len(m.Bits); i++ {
		if m.Bits[i] == from {
			m.Bits[i] = to
		}
	}
}

// statusCount returns the number of element that is in the target status
func (m *Mask) statusCount(status allocStatus) int {
	count := 0
	for i := 0; i < len(m.Bits); i++ {
		if m.Bits[i] == status {
			count++
		}
	}
	return count
}
