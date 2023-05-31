package noctracing

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rs/xid"
)

// Uint16 for node coordinate, [0, 65535]
// Uint32 for node ID, [0, 4294967295]
// Uint64 for message count, [0, ~1.8 * 10^19]
// Uint for time slice, on 64-bit machine equals to Uint64

// MeshNodeInfo is used to store mesh node info.
type MeshNodeInfo struct {
	// Data only for backend compution
	X uint16 `json:"-"`
	Y uint16 `json:"-"`

	// Data for JSON-format serialization to feed frontend as node info
	ID     uint32 `json:"id"`
	Name   string `json:"label,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// MeshEdgeInfo is used to store mesh edge info.
type MeshEdgeInfo struct {
	// Data only for backend compution
	Time2ValArray map[uint]([]uint64) `json:"-"`

	// Data for JSON-format serialization to feed frontend as edge info
	SourceID uint32   `json:"source"`
	TargetID uint32   `json:"target"`
	ValArray []uint64 `json:"value"`
	LinkName string   `json:"label,omitempty"`  // not used at present
	Detail   string   `json:"detail,omitempty"` // not used at present
}

// MeshMetaInfo is used to store mesh meta info (e.g., width, height).
type MeshMetaInfo struct {
	Width         string `json:"width"`
	Height        string `json:"height"`
	TimeSliceUnit string `json:"slice"` // time slice size of simulation
	Elapse        string `json:"elapse"`
}

// TwoDimMapForEdgeInfo maps [x, y] to the edge(x -> y) info.
type TwoDimMapForEdgeInfo = map[uint32]map[uint32](*MeshEdgeInfo)

// MeshInfo is used to store all info of a certain mesh.
type MeshInfo struct {
	width      uint16
	height     uint16
	elapse     uint
	edgeMatrix TwoDimMapForEdgeInfo // only used for tracer -> MeshInfo methods

	Meta     *MeshMetaInfo
	Nodes    []*MeshNodeInfo
	Edges    []*MeshEdgeInfo
	Overview map[uint]([]uint64)
}

// MeshOverviewInfoFrameToDump is used to store mesh overview.
// Note: only support time bar overview here.
type MeshOverviewInfoFrameToDump struct {
	FrameIdx         uint   `json:"id"`
	MsgExactType     string `json:"type"`
	MsgTypeGroup     string `json:"group"`
	MsgDataOrCommand string `json:"doc"`
	Count            uint64 `json:"count"`
	MaxFlits         uint64 `json:"max_flits"`
}

func (m *MeshInfo) initNodes() {
	col, row := m.width, m.height
	var i, j uint16
	var nodeIdx uint32
	for i = 0; i < row; i++ {
		for j = 0; j < col; j++ {
			nodeIdx = uint32(i)*uint32(col) + uint32(j)
			m.Nodes = append(m.Nodes, &MeshNodeInfo{
				X:      i,
				Y:      j,
				ID:     nodeIdx,
				Name:   fmt.Sprintf("Sw%d", nodeIdx),
				Detail: fmt.Sprintf("Sw[%d, %d]", i, j), // TODO: use node type
			})
			m.edgeMatrix[nodeIdx] = make(map[uint32]*MeshEdgeInfo)
		}
	}
}

func (m *MeshInfo) initEdgePool() {
	col, row := m.width, m.height
	var i, j uint16
	var cur uint32
	// Vertical direction
	for i = 1; i < row; i++ {
		for j = 0; j < col; j++ {
			cur = uint32(i)*uint32(col) + uint32(j)
			north := cur - uint32(col)
			forward := &MeshEdgeInfo{ // forward direction link
				Time2ValArray: make(map[uint]([]uint64)),
				SourceID:      north,
				TargetID:      cur,
				ValArray:      make([]uint64, 0),
				LinkName:      "",
				Detail:        "",
			}
			reverse := &MeshEdgeInfo{ // reversed direction link
				Time2ValArray: make(map[uint]([]uint64)),
				SourceID:      cur,
				TargetID:      north,
				ValArray:      make([]uint64, 0),
				LinkName:      "",
				Detail:        "",
			}
			m.edgeMatrix[north][cur] = forward
			m.edgeMatrix[cur][north] = reverse
			m.Edges = append(m.Edges, forward, reverse)
		}
	}
	// Horizontal direction
	for i = 0; i < row; i++ {
		for j = 1; j < col; j++ {
			cur = uint32(i)*uint32(col) + uint32(j)
			left := cur - 1
			forward := &MeshEdgeInfo{ // forward direction link
				Time2ValArray: make(map[uint]([]uint64)),
				SourceID:      left,
				TargetID:      cur,
				ValArray:      make([]uint64, 0),
				LinkName:      "",
				Detail:        "",
			}
			reverse := &MeshEdgeInfo{ // reversed direction link
				Time2ValArray: make(map[uint]([]uint64)),
				SourceID:      cur,
				TargetID:      left,
				ValArray:      make([]uint64, 0),
				LinkName:      "",
				Detail:        "",
			}
			m.edgeMatrix[left][cur] = forward
			m.edgeMatrix[cur][left] = reverse
			m.Edges = append(m.Edges, forward, reverse)
		}
	}
}

// refreshMeshInfoWithTimeRange sets values of MeshInfo.Edges to range of [from,
// to), if `from` is >= `to` then values would be reset to zeros.
func (m *MeshInfo) refreshMeshInfoWithTimeRange(from, to uint) {
	if from >= to {
		for _, e := range m.Edges {
			e.ValArray = make([]uint64, NumMeshMsgTypesToTrace) // reset to zeros
		}
	} else if from < 0 {
		panic("Only support non-negative time slice for refreshMeshInfoWithTimeRange")
	} else {
		for _, e := range m.Edges {
			e.ValArray = make([]uint64, NumMeshMsgTypesToTrace) // reset to zeros
			for time := from; time < to; time++ {
				for idx, val := range e.Time2ValArray[time] { // uint64 slice value add
					e.ValArray[idx] += val
				}
			}
		}
	}
}

func (m *MeshInfo) padBlankFrameAfterTimeEnd() {
	m.elapse++ // the previous m.elapse stores the maximum time slice

	// Add a blank mesh at the end of metrics
	for _, e := range m.Edges {
		if e.Time2ValArray[m.elapse] == nil {
			e.Time2ValArray[m.elapse] = make([]uint64, NumMeshMsgTypesToTrace)
		} else {
			panic("Unreachable code, maybe some weired bugs exist")
		}
	}
	// Weird out-of-range error is solved here
	m.Overview[m.elapse] = make([]uint64, NumMeshMsgTypesToTrace)

	m.elapse++ // now m.elapse indicates the length of timeline
}

func (m *MeshInfo) dumpEdgesToFiles(dir string) {
	// Dump Edges to [0-elapse).json
	dirRange := filepath.Join(dir, "range")
	if err := os.Mkdir(dirRange, 0755); err != nil {
		panic(err)
	}
	for i := uint(0); i < m.elapse; i++ {
		m.refreshMeshInfoWithTimeRange(i, i+1)
		writeStringToFile(
			string(dumpJSONToBytes(m.Edges)),
			filepath.Join(dirRange, fmt.Sprintf("%d.json", i)),
		)
	}

	// Dump Edges prefix sum to [0-elapse).json
	dirEdgePrefixSum := filepath.Join(dir, "edge_prefix_sum")
	if err := os.Mkdir(dirEdgePrefixSum, 0755); err != nil {
		panic(err)
	}

	for _, e := range m.Edges {
		e.ValArray = make([]uint64, NumMeshMsgTypesToTrace) // reset to zeros
	}

	for i := uint(0); i < m.elapse; i++ {
		for _, e := range m.Edges {
			for idx, val := range e.Time2ValArray[i] { // uint64 slice value add
				e.ValArray[idx] += val
			}
		}
		writeStringToFile(
			string(dumpJSONToBytes(m.Edges)),
			filepath.Join(dirEdgePrefixSum, fmt.Sprintf("%d.json", i)),
		)
	}
}

func (m *MeshInfo) calcTimelineMaxFlits() []uint64 {
	timelineMaxFlits := make([]uint64, m.elapse)

	for i := uint(0); i < m.elapse; i++ {
		maxNumFlits := uint64(0)
		for _, e := range m.Edges {
			numFlits := uint64(0)
			for _, f := range e.Time2ValArray[i] {
				numFlits += f
			}
			if numFlits > maxNumFlits {
				maxNumFlits = numFlits
			}
		}
		timelineMaxFlits[i] = maxNumFlits
	}

	// fmt.Println(timelineMaxFlits)
	return timelineMaxFlits
}

// DumpToFiles dumps MeshInfo to directory `meshmetrics` or something similar,
// this method is NOT reentrant due to `padBlankFrameAfterTimeEnd` operation.
func (m *MeshInfo) DumpToFiles(dir string) {
	if m.elapse == 0 {
		log.Println("No tracing data was collected by mesh NoC tracer")
		return
	}

	// Make directory for all dumped metrics
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		dir += "_" + xid.New().String()
		fmt.Printf("Directory to store mesh metrics existed, create new `%s` for "+
			"this round\n", dir)
	}

	if err := os.Mkdir(dir, 0755); err != nil {
		panic(err)
	}

	m.padBlankFrameAfterTimeEnd() // NOT reentrant function

	// Dump Meta to meta.json
	m.Meta.Elapse = fmt.Sprintf("%d", m.elapse)
	writeStringToFile(
		string(dumpJSONToBytes(m.Meta)), filepath.Join(dir, "meta.json"),
	)

	// Dump Nodes to nodes.json
	writeStringToFile(
		string(dumpJSONToBytes(m.Nodes)), filepath.Join(dir, "nodes.json"),
	)

	// Dump edges to ${EDGE_DIR}/[0-elapse).json
	m.dumpEdgesToFiles(dir)
	timelineMaxFlits := m.calcTimelineMaxFlits()

	// Dump Overview to flat.json
	snapshots := make(
		[]MeshOverviewInfoFrameToDump, m.elapse*uint(NumMeshMsgTypesToTrace),
	)
	for i := uint(0); i < m.elapse; i++ {
		offset := i * uint(NumMeshMsgTypesToTrace)
		for j := 0; j < NumMeshMsgTypesToTrace; j++ {
			name := MeshMsgTypesToTraceList[j]
			snapshots[offset+uint(j)] = MeshOverviewInfoFrameToDump{
				FrameIdx:         i,
				MsgExactType:     name,
				MsgTypeGroup:     MeshMsgTypesGroupMap[name],
				MsgDataOrCommand: MeshMsgDataOrCommandMap[name],
				Count:            m.Overview[i][j],
				MaxFlits:         timelineMaxFlits[i],
			}
		}
	}
	writeStringToFile(
		string(dumpJSONToBytes(snapshots)), filepath.Join(dir, "flat.json"),
	)

	log.Println("Mesh metrics dumped successfully")
}

// AppendEdgeInfo appends edge info, then update edge matrix and count buffers.
func (m *MeshInfo) AppendEdgeInfo(
	time uint, srcTile, dstTile [3]int, msgTypeID uint8,
) {
	src := uint32(srcTile[0]*int(m.width) + srcTile[1])
	dst := uint32(dstTile[0]*int(m.width) + dstTile[1])
	if time > m.elapse {
		m.elapse = time
	}
	e := m.edgeMatrix[src][dst]
	if e.Time2ValArray[time] == nil {
		e.Time2ValArray[time] = make([]uint64, NumMeshMsgTypesToTrace)
	}
	e.Time2ValArray[time][msgTypeID]++
}

// AppendOverviewInfo appends overview with time slice and msg type.
func (m *MeshInfo) AppendOverviewInfo(time uint, msgTypeID uint8) {
	if m.Overview[time] == nil {
		m.Overview[time] = make([]uint64, NumMeshMsgTypesToTrace)
	}
	m.Overview[time][msgTypeID]++
}

// MakeMeshInfo builds a MeshInfo struct with some arguments as meta info.
func MakeMeshInfo(width, height uint16, timeSliceUnit float64) *MeshInfo {
	m := new(MeshInfo)
	m.width = width
	m.height = height
	m.elapse = 0 // m.elapse would update when new edge info record added
	m.edgeMatrix = make(TwoDimMapForEdgeInfo)

	m.Meta = &MeshMetaInfo{
		Width:         fmt.Sprintf("%d", width),
		Height:        fmt.Sprintf("%d", height),
		TimeSliceUnit: fmt.Sprintf("%.9f", timeSliceUnit),
	}
	m.Overview = make(map[uint][]uint64)

	m.initNodes()
	m.initEdgePool()

	return m
}

//
// Utility Functions
//

func writeStringToFile(data, file string) {
	f, err := os.Create(file)
	if err != nil {
		log.Fatal(err)
	}

	writer := bufio.NewWriter(f)
	defer f.Close()

	fmt.Fprintln(writer, data)
	writer.Flush()
}

func dumpJSONToBytes(v interface{}) []byte {
	output, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return output
}

func checkFileExist(file string) bool {
	_, err := os.Stat(file)
	return !os.IsNotExist(err)
}

func readStringFileToJSON(file string, v interface{}) {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		panic(err)
	}
	// pointer of pointer is allowed in the 2rd parameter of `json.Unmarshal`
	if err := json.Unmarshal(content, &v); err != nil {
		panic(err)
	}
}

func parseUintDecimal(str string) uint {
	result, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		panic(err)
	}
	return uint(result)
}

func parseUint16Decimal(str string) uint16 {
	result, err := strconv.ParseUint(str, 10, 16)
	if err != nil {
		panic(err)
	}
	return uint16(result)
}

//
// Message Types to Trace
//

// Note: *protocol.MemCopyD2HReq, *protocol.MemCopyH2DReq would appear in mesh
//       network, so we don't include them in the following message type list.

// NumMeshMsgTypesToTrace is the total number of msg types to trace.
var NumMeshMsgTypesToTrace = 12

// MeshMsgTypesToTrace maps the msg type string to an enumerated ID.
var MeshMsgTypesToTrace = map[string]uint8{
	"*cache.FlushReq":           0,
	"*cache.FlushRsp":           1,
	"*mem.DataReadyRsp":         2,
	"*mem.ReadReq":              3,
	"*mem.WriteDoneRsp":         4,
	"*mem.WriteReq":             5,
	"*protocol.FlushReq":        6,
	"*protocol.LaunchKernelReq": 7,
	"*protocol.MapWGReq":        8,
	"*protocol.WGCompletionMsg": 9,
	"*vm.TranslationReq":        10,
	"*vm.TranslationRsp":        11,
}

// MeshMsgTypesToTraceList is a slice of the msg type strings.
var MeshMsgTypesToTraceList = []string{
	"*cache.FlushReq",
	"*cache.FlushRsp",
	"*mem.DataReadyRsp",
	"*mem.ReadReq",
	"*mem.WriteDoneRsp",
	"*mem.WriteReq",
	"*protocol.FlushReq",
	"*protocol.LaunchKernelReq",
	"*protocol.MapWGReq",
	"*protocol.WGCompletionMsg",
	"*vm.TranslationReq",
	"*vm.TranslationRsp",
}

// MeshMsgTypesGroupMap maps the msg type string to a classification, like Read/
// Write/Translation/Others.
var MeshMsgTypesGroupMap = map[string]string{
	"*cache.FlushReq":           "Others",
	"*cache.FlushRsp":           "Others",
	"*mem.DataReadyRsp":         "Read",
	"*mem.ReadReq":              "Read",
	"*mem.WriteDoneRsp":         "Write",
	"*mem.WriteReq":             "Write",
	"*protocol.FlushReq":        "Others",
	"*protocol.LaunchKernelReq": "Others",
	"*protocol.MapWGReq":        "Others",
	"*protocol.WGCompletionMsg": "Others",
	"*vm.TranslationReq":        "Translation",
	"*vm.TranslationRsp":        "Translation",
}

// MeshMsgDataOrCommandMap maps the msg type string to a binary classification,
// i.e., data or command msg.
var MeshMsgDataOrCommandMap = map[string]string{
	"*cache.FlushReq":           "C",
	"*cache.FlushRsp":           "C",
	"*mem.DataReadyRsp":         "D",
	"*mem.ReadReq":              "C",
	"*mem.WriteDoneRsp":         "C",
	"*mem.WriteReq":             "D",
	"*protocol.FlushReq":        "C",
	"*protocol.LaunchKernelReq": "C",
	"*protocol.MapWGReq":        "C",
	"*protocol.WGCompletionMsg": "C",
	"*vm.TranslationReq":        "C",
	"*vm.TranslationRsp":        "D",
}
