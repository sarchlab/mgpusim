package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "github.com/glebarez/go-sqlite"
	"github.com/sarchlab/akita/v5/timing"
)

func (s *Server) httpTrace(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	useTimeRange := true
	if r.FormValue("starttime") == "" || r.FormValue("endtime") == "" {
		useTimeRange = false
	}

	var err error

	startTime := 0.0
	endTime := 0.0

	if useTimeRange {
		startTime, err = strconv.ParseFloat(r.FormValue("starttime"), 64)
		if err != nil {
			panic(err)
		}

		endTime, err = strconv.ParseFloat(r.FormValue("endtime"), 64)
		if err != nil {
			panic(err)
		}
	}

	var queryID uint64
	if idStr := r.FormValue("id"); idStr != "" {
		queryID, _ = strconv.ParseUint(idStr, 10, 64)
	}
	var queryParentID uint64
	if pidStr := r.FormValue("parentid"); pidStr != "" {
		queryParentID, _ = strconv.ParseUint(pidStr, 10, 64)
	}

	query := TaskQuery{
		ID:               queryID,
		ParentID:         queryParentID,
		Kind:             r.FormValue("kind"),
		Where:            r.FormValue("where"),
		StartTime:        startTime,
		EndTime:          endTime,
		EnableTimeRange:  useTimeRange,
		EnableParentTask: false,
		EnableMilestones: true,
	}

	tasks := s.traceReader.ListTasks(r.Context(), query)

	rsp, err := json.Marshal(tasks)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}

// TaskQuery is used to define the tasks to be queried. Not all the field has to
// be set. If the fields are empty, the criteria is ignored.
type TaskQuery struct {
	// Use ID to select a single task by its ID.
	ID uint64

	// Use ParentID to select all the tasks that are children of a task.
	ParentID uint64

	// Use Kind to select all the tasks that are of a kind.
	Kind string

	// Use Where to select all the tasks that are executed at a location.
	Where string

	// Enable time range selection.
	EnableTimeRange bool

	// Use StartTime to select tasks that overlaps with the given task range.
	StartTime, EndTime float64

	// EnableParentTask will also query the parent task of the selected tasks.
	EnableParentTask bool

	// EnableMilestones will also query milestones for the selected tasks.
	EnableMilestones bool
}

// TaskStep represents a milestone/step in a task.
type TaskStep struct {
	Time timing.VTimeInPicoSec `json:"time"`
	What string                `json:"what"`
	Kind string                `json:"kind"`
}

// Task represents a traced task.
type Task struct {
	ID         uint64                `json:"id"`
	ParentID   uint64                `json:"parent_id"`
	Kind       string                `json:"kind"`
	What       string                `json:"what"`
	Location   string                `json:"location"`
	StartTime  timing.VTimeInPicoSec `json:"start_time"`
	EndTime    timing.VTimeInPicoSec `json:"end_time"`
	Steps      []TaskStep            `json:"steps"`
	Detail     interface{}           `json:"-"`
	ParentTask *Task                 `json:"-"`
}

// TraceReader can parse a trace file.
type TraceReader interface {
	// ListComponents returns all the locations used in the trace.
	ListComponents(ctx context.Context) []string

	// ListTasks queries tasks .
	ListTasks(ctx context.Context, query TaskQuery) []Task
}

// TraceTimeRange is the full time span covered by the trace table.
type TraceTimeRange struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// SQLiteTraceReader is a reader that reads trace data from a SQLite database.
type SQLiteTraceReader struct {
	*sql.DB

	filename string
}

// NewSQLiteTraceReader creates a new SQLiteTraceReader.
func NewSQLiteTraceReader(filename string) *SQLiteTraceReader {
	r := &SQLiteTraceReader{
		filename: filename,
	}

	return r
}

// Init establishes a connection to the database.
func (r *SQLiteTraceReader) Init() {
	db, err := sql.Open("sqlite", r.filename)
	if err != nil {
		panic(err)
	}

	// Enable WAL mode for concurrent read access.
	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		panic(err)
	}

	r.DB = db
}

// InitReadOnly establishes a read-only connection to the database with WAL mode.
func (r *SQLiteTraceReader) InitReadOnly() {
	db, err := sql.Open("sqlite", r.filename+"?mode=ro")
	if err != nil {
		panic(err)
	}

	// Enable WAL mode for concurrent read access.
	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		panic(err)
	}

	r.DB = db
}

func naturalLess(a, b string) bool {
	re := regexp.MustCompile(`\d+|\D+`)
	as := re.FindAllString(a, -1)
	bs := re.FindAllString(b, -1)

	for i := 0; i < len(as) && i < len(bs); i++ {
		anum, aErr := strconv.Atoi(as[i])
		bnum, bErr := strconv.Atoi(bs[i])

		if aErr == nil && bErr == nil {
			if anum != bnum {
				return anum < bnum
			}
		} else {
			if as[i] != bs[i] {
				return as[i] < bs[i]
			}
		}
	}

	return len(as) < len(bs)
}

// ListComponents returns a list of components in the trace.
func (r *SQLiteTraceReader) ListComponents(ctx context.Context) []string {
	var components []string

	// The shared location table holds exactly the set of component names used
	// in the trace, each interned once.
	rows, err := r.QueryContext(ctx, "SELECT Locale FROM location")
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		panic(err)
	}

	defer func() {
		err := rows.Close()
		if err != nil && ctx.Err() == nil {
			panic(err)
		}
	}()

	for rows.Next() {
		var component string

		err := rows.Scan(&component)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			panic(err)
		}

		components = append(components, component)
	}

	sort.Slice(components, func(i, j int) bool {
		return naturalLess(components[i], components[j])
	})

	return components
}

// ListTasks returns a list of tasks in the trace according to the given query.
func (r *SQLiteTraceReader) ListTasks(ctx context.Context, query TaskQuery) []Task {
	sqlStr := r.prepareTaskQueryStr(query)

	rows, err := r.QueryContext(ctx, sqlStr)
	if err != nil {
		panic(err)
	}

	defer rows.Close()

	tasks := []Task{}

	for rows.Next() {
		task := r.scanTaskFromRow(rows, query.EnableParentTask)
		tasks = append(tasks, task)
	}

	if query.EnableMilestones {
		r.loadMilestonesForTasks(tasks)
		r.loadTagsForTasks(tasks)
		sortTaskSteps(tasks)
	}

	return tasks
}

// sortTaskSteps orders each task's Steps by time, so milestones and tags loaded
// from separate tables form one coherent timeline.
func sortTaskSteps(tasks []Task) {
	for i := range tasks {
		steps := tasks[i].Steps
		sort.SliceStable(steps, func(a, b int) bool {
			return steps[a].Time < steps[b].Time
		})
	}
}

// TimeRange returns the min task start time and max task end time in the trace.
func (r *SQLiteTraceReader) TimeRange(ctx context.Context) (TraceTimeRange, bool) {
	if timeRange, ok := r.execInfoTimeRange(ctx); ok {
		return timeRange, true
	}

	row := r.QueryRowContext(ctx, "SELECT MIN(StartTime), MAX(EndTime) FROM trace")

	var startTime, endTime sql.NullFloat64
	err := row.Scan(&startTime, &endTime)
	if err != nil {
		if ctx.Err() != nil {
			return TraceTimeRange{}, false
		}
		panic(err)
	}

	if !startTime.Valid || !endTime.Valid || startTime.Float64 >= endTime.Float64 {
		return TraceTimeRange{}, false
	}

	return TraceTimeRange{
		StartTime: startTime.Float64,
		EndTime:   endTime.Float64,
	}, true
}

func (r *SQLiteTraceReader) execInfoTimeRange(ctx context.Context) (TraceTimeRange, bool) {
	rows, err := r.QueryContext(ctx, `
		SELECT Property, Value
		FROM exec_info
		WHERE Property IN ('Start Virtual Time', 'End Virtual Time')
	`)
	if err != nil {
		return TraceTimeRange{}, false
	}
	defer rows.Close()

	var timeRange TraceTimeRange
	var hasStart, hasEnd bool
	for rows.Next() {
		var property, value string
		err := rows.Scan(&property, &value)
		if err != nil {
			return TraceTimeRange{}, false
		}

		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return TraceTimeRange{}, false
		}

		switch property {
		case "Start Virtual Time":
			timeRange.StartTime = parsed
			hasStart = true
		case "End Virtual Time":
			timeRange.EndTime = parsed
			hasEnd = true
		}
	}

	if !hasStart || !hasEnd || timeRange.StartTime >= timeRange.EndTime {
		return TraceTimeRange{}, false
	}

	return timeRange, true
}

func (s *Server) httpTraceTimeRange(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	timeRange, ok := s.traceReader.TimeRange(r.Context())
	if !ok {
		http.Error(w, "trace time range not available", http.StatusNotFound)
		return
	}

	rsp, err := json.Marshal(timeRange)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}

// loadMilestonesForTasks loads milestones for the given tasks from the database.
func (r *SQLiteTraceReader) loadMilestonesForTasks(tasks []Task) {
	if len(tasks) == 0 {
		return
	}

	// Build a map for quick task lookup
	taskMap := make(map[uint64]*Task)
	taskIDs := make([]interface{}, 0, len(tasks))
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
		taskIDs = append(taskIDs, tasks[i].ID)
	}

	// Query milestones for all tasks using parameterized query
	placeholders := strings.Repeat("?,", len(taskIDs))
	if len(placeholders) > 0 {
		placeholders = placeholders[:len(placeholders)-1] // remove trailing comma
	}

	// A milestone's location is inherited from its task, so the milestone
	// table no longer stores it; we read the remaining columns only.
	sqlStr := fmt.Sprintf(`
		SELECT TaskID, Time, Kind, What
		FROM milestone
		WHERE TaskID IN (%s)
		ORDER BY TaskID, Time`, placeholders)

	rows, err := r.Query(sqlStr, taskIDs...)
	if err != nil {
		// If milestone table doesn't exist, just return without error
		return
	}
	defer rows.Close()

	for rows.Next() {
		var taskID uint64
		var kind, what string
		var time float64

		err := rows.Scan(&taskID, &time, &kind, &what)
		if err != nil {
			continue
		}

		if task, exists := taskMap[taskID]; exists {
			step := TaskStep{
				Time: timing.VTimeInPicoSec(uint64(time)),
				What: what,
				Kind: kind,
			}
			task.Steps = append(task.Steps, step)
		}
	}
}

// loadTagsForTasks loads the categorical tags persisted in the tag table for
// the given tasks and merges them into each task's Steps stream alongside
// milestones. A tag's location is inherited from its task, so the tag table
// stores none; tags also carry no Kind, so they are labelled "tag" to stay
// distinguishable from milestones in the merged stream.
func (r *SQLiteTraceReader) loadTagsForTasks(tasks []Task) {
	if len(tasks) == 0 {
		return
	}

	taskMap := make(map[uint64]*Task)
	taskIDs := make([]interface{}, 0, len(tasks))
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
		taskIDs = append(taskIDs, tasks[i].ID)
	}

	placeholders := strings.Repeat("?,", len(taskIDs))
	if len(placeholders) > 0 {
		placeholders = placeholders[:len(placeholders)-1] // remove trailing comma
	}

	sqlStr := fmt.Sprintf(`
		SELECT TaskID, Time, What
		FROM tag
		WHERE TaskID IN (%s)
		ORDER BY TaskID, Time`, placeholders)

	rows, err := r.Query(sqlStr, taskIDs...)
	if err != nil {
		// If the tag table doesn't exist, just return without error.
		return
	}
	defer rows.Close()

	for rows.Next() {
		var taskID uint64
		var what string
		var time float64

		err := rows.Scan(&taskID, &time, &what)
		if err != nil {
			continue
		}

		if task, exists := taskMap[taskID]; exists {
			task.Steps = append(task.Steps, TaskStep{
				Time: timing.VTimeInPicoSec(uint64(time)),
				What: what,
				Kind: "tag",
			})
		}
	}
}

func (r *SQLiteTraceReader) scanTaskFromRow(
	rows *sql.Rows,
	enableParentTask bool,
) Task {
	t := Task{}

	if enableParentTask {
		t.ParentTask = &Task{}
		r.scanTaskWithParent(rows, &t)
	} else {
		r.scanTaskWithoutParent(rows, &t)
	}

	return t
}

func (r *SQLiteTraceReader) scanTaskWithParent(rows *sql.Rows, t *Task) {
	var ptID, ptParentID sql.NullInt64
	var ptKind, ptWhat, ptLocation sql.NullString
	var ptStartTime, ptEndTime sql.NullFloat64
	var startTime, endTime float64

	err := rows.Scan(
		&t.ID,
		&t.ParentID,
		&t.Kind,
		&t.What,
		&t.Location,
		&startTime,
		&endTime,
		&ptID,
		&ptParentID,
		&ptKind,
		&ptWhat,
		&ptLocation,
		&ptStartTime,
		&ptEndTime,
	)
	if err != nil {
		panic(err)
	}

	t.StartTime = timing.VTimeInPicoSec(uint64(startTime))
	t.EndTime = timing.VTimeInPicoSec(uint64(endTime))

	if ptID.Valid {
		t.ParentTask.ID = uint64(ptID.Int64)
		t.ParentTask.ParentID = uint64(ptParentID.Int64)
		t.ParentTask.Kind = ptKind.String
		t.ParentTask.What = ptWhat.String
		t.ParentTask.Location = ptLocation.String
		t.ParentTask.StartTime = timing.VTimeInPicoSec(uint64(ptStartTime.Float64))
		t.ParentTask.EndTime = timing.VTimeInPicoSec(uint64(ptEndTime.Float64))
	}
}

func (r *SQLiteTraceReader) scanTaskWithoutParent(rows *sql.Rows, t *Task) {
	var startTime, endTime float64

	err := rows.Scan(
		&t.ID,
		&t.ParentID,
		&t.Kind,
		&t.What,
		&t.Location,
		&startTime,
		&endTime,
	)
	if err != nil {
		panic(err)
	}

	t.StartTime = timing.VTimeInPicoSec(uint64(startTime))
	t.EndTime = timing.VTimeInPicoSec(uint64(endTime))
}

func (r *SQLiteTraceReader) prepareTaskQueryStr(query TaskQuery) string {
	// Location is stored as an integer id that references the shared location
	// table; join it back to the component name string.
	sqlStr := `
		SELECT
			t.ID,
			t.ParentID,
			t.Kind,
			t.What,
			loc.Locale,
			t.StartTime,
			t.EndTime
	`

	if query.EnableParentTask {
		sqlStr += `,
			pt.ID,
			pt.ParentID,
			pt.Kind,
			pt.What,
			ploc.Locale,
			pt.StartTime,
			pt.EndTime
		`
	}

	sqlStr += `
		FROM trace t
		JOIN location loc ON t.Location = loc.ID
	`

	if query.EnableParentTask {
		sqlStr += `
			LEFT JOIN trace pt
			ON t.ParentID = pt.ID
			LEFT JOIN location ploc
			ON pt.Location = ploc.ID
		`
	}

	sqlStr = r.addQueryConditionsToQueryStr(sqlStr, query)

	return sqlStr
}

func (*SQLiteTraceReader) addQueryConditionsToQueryStr(
	sqlStr string,
	query TaskQuery,
) string {
	sqlStr += `
		WHERE 1=1
	`

	if query.ID != 0 {
		sqlStr += `
			AND t.ID = ` + strconv.FormatUint(query.ID, 10) + `
		`
	}

	if query.ParentID != 0 {
		sqlStr += `
			AND t.ParentID = ` + strconv.FormatUint(query.ParentID, 10) + `
		`
	}

	if query.Kind != "" {
		sqlStr += `
			AND t.Kind = '` + query.Kind + `'
		`
	}

	if query.Where != "" {
		sqlStr += `
			AND loc.Locale = '` + query.Where + `'
		`
	}

	if query.EnableTimeRange {
		sqlStr += fmt.Sprintf(
			"AND t.EndTime > %.15f AND t.StartTime < %.15f",
			query.StartTime,
			query.EndTime)
	}

	return sqlStr
}

// Segment represents a time segment where traces were collected
type Segment struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// SegmentsResponse contains the segments data and whether the feature is enabled
type SegmentsResponse struct {
	Enabled  bool      `json:"enabled"`
	Segments []Segment `json:"segments"`
}

// HasSegmentsTable checks if the daisen$segments table exists in the database
func (r *SQLiteTraceReader) HasSegmentsTable(ctx context.Context) bool {
	query := `SELECT name FROM sqlite_master WHERE type='table' AND name='daisen$segments'`
	rows, err := r.QueryContext(ctx, query)
	if err != nil {
		return false
	}
	defer rows.Close()

	return rows.Next()
}

// ListSegments returns all segments from the daisen$segments table
func (r *SQLiteTraceReader) ListSegments(ctx context.Context) SegmentsResponse {
	response := SegmentsResponse{
		Enabled:  false,
		Segments: []Segment{},
	}

	if !r.HasSegmentsTable(ctx) {
		return response
	}

	response.Enabled = true

	query := `SELECT StartTime, EndTime FROM "daisen$segments" ORDER BY StartTime`
	rows, err := r.QueryContext(ctx, query)
	if err != nil {
		return response
	}
	defer rows.Close()

	for rows.Next() {
		var segment Segment
		err := rows.Scan(&segment.StartTime, &segment.EndTime)
		if err != nil {
			continue
		}
		response.Segments = append(response.Segments, segment)
	}

	return response
}

func (s *Server) httpSegments(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	segments := s.traceReader.ListSegments(r.Context())

	rsp, err := json.Marshal(segments)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}
