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

// A resourceMask marks which part of the resource is use.
type resourceMask interface {
	// nextRegion finds a region that is masked by the resourceMask in the state
	// defined by desiredStatus. This function returns the offset of the
	// starting point of the region. It also returns a boolean value that tells
	// if such a region is found.
	nextRegion(length int, desiredStatus allocStatus) (int, bool)

	// setStatus sets a range of bits to status
	setStatus(offset, length int, status allocStatus)

	// convertStatus converts status of one type to another.
	convertStatus(from, to allocStatus)

	// statusCount returns the number of bits of a given status.
	statusCount(status allocStatus) int
}

// An unlimitedResourceMask is a mask for unlimited resources.
type unlimitedResourceMask struct {
	next int
}

func (m *unlimitedResourceMask) nextRegion(
	length int,
	statusReq allocStatus,
) (int, bool) {
	point := m.next

	m.next += length

	return point, true
}

func (m *unlimitedResourceMask) setStatus(
	offset, length int,
	status allocStatus,
) {
	// Do nothing
}

func (m *unlimitedResourceMask) convertStatus(from, to allocStatus) {
	// Do nothing
}

func (m *unlimitedResourceMask) statusCount(status allocStatus) int {
	return 0
}

// A resourceMaskImpl is data structure to mask the status of some resources
type resourceMaskImpl struct {
	mask []allocStatus
}

// newResourceMask returns a newly created ResourceMask with a given size.
func newResourceMask(size int) *resourceMaskImpl {
	m := new(resourceMaskImpl)
	m.mask = make([]allocStatus, size)
	return m
}

// nextRegion finds a region that is masked by the resourceMask in
// the state define by statusReq. This function returns the offset of the
// starting point of the region. It also returns a boolean value that
// tells if a region is found
func (m *resourceMaskImpl) nextRegion(
	length int,
	statusReq allocStatus,
) (int, bool) {
	if length == 0 {
		return 0, true
	}

	offset := 0
	currLength := 0
	for offset < len(m.mask) {
		if m.mask[offset] == statusReq {
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
func (m *resourceMaskImpl) setStatus(offset, length int, status allocStatus) {
	for i := 0; i < length; i++ {
		m.mask[offset+i] = status
	}
}

// convertStatus change all the element of one status to another
func (m *resourceMaskImpl) convertStatus(from, to allocStatus) {
	for i := 0; i < len(m.mask); i++ {
		if m.mask[i] == from {
			m.mask[i] = to
		}
	}
}

// statusCount returns the number of element that is in the target status
func (m *resourceMaskImpl) statusCount(status allocStatus) int {
	count := 0
	for i := 0; i < len(m.mask); i++ {
		if m.mask[i] == status {
			count++
		}
	}
	return count
}
