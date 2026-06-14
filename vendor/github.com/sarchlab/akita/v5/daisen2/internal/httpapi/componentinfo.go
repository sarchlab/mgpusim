package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// TimeValue represents a data point with a time and a value.
type TimeValue struct {
	Time  float64 `json:"time"`
	Value float64 `json:"value"`
}

// ComponentInfo contains time-series info for a component.
type ComponentInfo struct {
	Name      string      `json:"name"`
	InfoType  string      `json:"info_type"`
	StartTime float64     `json:"start_time"`
	EndTime   float64     `json:"end_time"`
	Data      []TimeValue `json:"data"`
}

// StackedTimeValue represents a data point with per-kind values.
type StackedTimeValue struct {
	Time   float64            `json:"time"`
	Values map[string]float64 `json:"values"` // key: milestone kind, value: count
}

// StackedComponentInfo contains stacked time-series info for a component.
type StackedComponentInfo struct {
	Name      string             `json:"name"`
	InfoType  string             `json:"info_type"`
	StartTime float64            `json:"start_time"`
	EndTime   float64            `json:"end_time"`
	Data      []StackedTimeValue `json:"data"`
	Kinds     []string           `json:"kinds"` // list of all milestone kinds
}

func (s *Server) httpComponentNames(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	componentNames := s.traceReader.ListComponents(r.Context())

	rsp, err := json.Marshal(componentNames)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}

func (s *Server) httpComponentInfo(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	compName := r.FormValue("where")
	infoType := r.FormValue("info_type")

	startTime, err := strconv.ParseFloat(r.FormValue("start_time"), 64)
	dieOnErr(err)

	endTime, err := strconv.ParseFloat(r.FormValue("end_time"), 64)
	dieOnErr(err)

	numDots, err := strconv.ParseInt(r.FormValue("num_dots"), 10, 32)
	dieOnErr(err)

	var compInfo *ComponentInfo

	switch infoType {
	case "ReqInCount":
		compInfo = s.calculateReqIn(
			r.Context(), compName, startTime, endTime, int(numDots))
	case "ReqCompleteCount":
		compInfo = s.calculateReqComplete(
			r.Context(), compName, startTime, endTime, int(numDots))
	case "AvgLatency":
		compInfo = s.calculateAvgLatency(
			r.Context(), compName, startTime, endTime, int(numDots))
	case "ConcurrentTask":
		compInfo = s.calculateConcurrentTask(
			r.Context(), compInfo, compName, infoType, startTime, endTime, numDots)
	case "ConcurrentTaskMilestones":
		stackedInfo := s.calculateConcurrentTaskMilestones(
			r.Context(), compName, infoType, startTime, endTime, int(numDots))
		rsp, err := json.Marshal(stackedInfo)
		dieOnErr(err)
		_, err = w.Write(rsp)
		dieOnErr(err)
		return
	case "BufferPressure":
		compInfo = s.calculateBufferPressure(
			r.Context(), compInfo, compName, infoType, startTime, endTime, numDots)
	case "PendingReqOut":
		compInfo = s.calculatePendingReqOut(
			r.Context(), compInfo, compName, infoType, startTime, endTime, numDots)
	default:
		log.Panicf("unknown info_type %s\n", infoType)
	}

	rsp, err := json.Marshal(compInfo)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}

func (s *Server) calculateConcurrentTask(
	ctx context.Context,
	compInfo *ComponentInfo,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	compInfo = s.calculateTimeWeightedTaskCount(
		ctx, compName, infoType,
		startTime, endTime, int(numDots),
		false,
		func(t Task) bool { return true },
		func(t Task) float64 { return float64(t.StartTime) },
		func(t Task) float64 { return float64(t.EndTime) },
	)

	return compInfo
}

func (s *Server) calculateBufferPressure(
	ctx context.Context,
	compInfo *ComponentInfo,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	compInfo = s.calculateTimeWeightedTaskCount(
		ctx, compName, infoType,
		startTime, endTime, int(numDots),
		true,
		taskIsReqIn,
		func(t Task) float64 {
			return float64(t.ParentTask.StartTime)
		},
		func(t Task) float64 {
			return float64(t.StartTime)
		},
	)

	return compInfo
}

func (s *Server) calculatePendingReqOut(
	ctx context.Context,
	compInfo *ComponentInfo,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	compInfo = s.calculateTimeWeightedTaskCount(
		ctx, compName, infoType,
		startTime, endTime, int(numDots),
		false,
		func(t Task) bool { return t.Kind == "req_out" },
		func(t Task) float64 { return float64(t.StartTime) },
		func(t Task) float64 { return float64(t.EndTime) },
	)

	return compInfo
}

func taskIsReqIn(t Task) bool {
	return t.Kind == "req_in" && t.ParentTask != nil
}

func (s *Server) calculateReqIn(
	ctx context.Context,
	compName string,
	startTime, endTime float64,
	numDots int,
) *ComponentInfo {
	info := &ComponentInfo{
		Name:      compName,
		InfoType:  "req_in",
		StartTime: startTime,
		EndTime:   endTime,
	}

	totalDuration := endTime - startTime
	if numDots <= 0 || totalDuration <= 0 {
		return info
	}

	binDuration := totalDuration / float64(numDots)
	info.Data = buildEmptyTimeValues(startTime, binDuration, numDots)

	if binDuration <= 0 {
		return info
	}

	s.fillBinnedEventRate(ctx, info.Data,
		compName, "req_in", "StartTime",
		startTime, endTime, binDuration)

	return info
}

func (s *Server) calculateReqComplete(
	ctx context.Context,
	compName string,
	startTime, endTime float64,
	numDots int,
) *ComponentInfo {
	info := &ComponentInfo{
		Name:      compName,
		InfoType:  "req_complete",
		StartTime: startTime,
		EndTime:   endTime,
	}

	totalDuration := endTime - startTime
	if numDots <= 0 || totalDuration <= 0 {
		return info
	}

	binDuration := totalDuration / float64(numDots)
	info.Data = buildEmptyTimeValues(startTime, binDuration, numDots)

	if binDuration <= 0 {
		return info
	}

	s.fillBinnedEventRate(ctx, info.Data,
		compName, "req_in", "EndTime",
		startTime, endTime, binDuration)

	return info
}

func (s *Server) calculateAvgLatency(
	ctx context.Context,
	compName string,
	startTime, endTime float64,
	numDots int,
) *ComponentInfo {
	info := &ComponentInfo{
		Name:      compName,
		InfoType:  "avg_latency",
		StartTime: startTime,
		EndTime:   endTime,
	}

	totalDuration := endTime - startTime
	if numDots <= 0 || totalDuration <= 0 {
		return info
	}

	binDuration := totalDuration / float64(numDots)
	info.Data = buildEmptyTimeValues(startTime, binDuration, numDots)

	if binDuration <= 0 {
		return info
	}

	s.fillBinnedAverageLatency(ctx, info.Data,
		compName, startTime, endTime, binDuration)

	return info
}

func buildEmptyTimeValues(startTime, binDuration float64, numDots int) []TimeValue {
	data := make([]TimeValue, numDots)
	for i := range data {
		binStartTime := float64(i)*binDuration + startTime
		data[i] = TimeValue{
			Time: binStartTime + 0.5*binDuration,
		}
	}
	return data
}

func (s *Server) fillBinnedEventRate(
	ctx context.Context,
	data []TimeValue,
	compName, kind, timeColumn string,
	startTime, endTime, binDuration float64,
) {
	timeColumn = safeTraceTimeColumn(timeColumn)
	query := fmt.Sprintf(`
		SELECT CAST(((%s - ?) / ?) AS INTEGER) AS Bin, COUNT(*)
		FROM trace
		WHERE Location = (SELECT ID FROM location WHERE Locale = ?)
			AND Kind = ? AND %s > ? AND %s < ?
		GROUP BY Bin
	`, timeColumn, timeColumn, timeColumn)

	rows, err := s.traceReader.QueryContext(
		ctx, query,
		startTime, binDuration,
		compName, kind, startTime, endTime,
	)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var bin int
		var count int64
		if err := rows.Scan(&bin, &count); err != nil {
			if ctx.Err() != nil {
				return
			}
			panic(err)
		}

		if bin >= 0 && bin < len(data) {
			data[bin].Value = float64(count) / binDuration
		}
	}
}

func (s *Server) fillBinnedAverageLatency(
	ctx context.Context,
	data []TimeValue,
	compName string,
	startTime, endTime, binDuration float64,
) {
	query := `
		SELECT CAST(((EndTime - ?) / ?) AS INTEGER) AS Bin,
			AVG(EndTime - StartTime)
		FROM trace
		WHERE Location = (SELECT ID FROM location WHERE Locale = ?)
			AND Kind = 'req_in' AND EndTime > ? AND EndTime < ?
		GROUP BY Bin
	`

	rows, err := s.traceReader.QueryContext(
		ctx, query,
		startTime, binDuration,
		compName, startTime, endTime,
	)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var bin int
		var value float64
		if err := rows.Scan(&bin, &value); err != nil {
			if ctx.Err() != nil {
				return
			}
			panic(err)
		}

		if bin >= 0 && bin < len(data) {
			data[bin].Value = value
		}
	}
}

func safeTraceTimeColumn(column string) string {
	switch column {
	case "StartTime", "EndTime":
		return column
	default:
		panic(fmt.Sprintf("unsupported trace time column %q", column))
	}
}

type taskFilter func(t Task) bool
type taskTime func(t Task) float64

func (s *Server) calculateTimeWeightedTaskCount(
	ctx context.Context,
	compName, infoType string,
	startTime, endTime float64,
	numDots int,
	enableParentTask bool,
	filter taskFilter,
	increaseTime, decreaseTime taskTime,
) *ComponentInfo {
	info := &ComponentInfo{
		Name:      compName,
		InfoType:  infoType,
		StartTime: startTime,
		EndTime:   endTime,
	}

	query := TaskQuery{
		Where:            compName,
		EnableTimeRange:  true,
		StartTime:        startTime,
		EndTime:          endTime,
		EnableParentTask: enableParentTask,
	}
	tasks := s.traceReader.ListTasks(ctx, query)

	totalDuration := endTime - startTime
	if numDots <= 0 || totalDuration <= 0 {
		return info
	}

	binDuration := totalDuration / float64(numDots)
	info.Data = calculateTimeWeightedBins(
		tasks, startTime, endTime, binDuration, numDots,
		filter, increaseTime, decreaseTime)

	return info
}

type countEvent struct {
	time  float64
	delta int
}

func calculateTimeWeightedBins( //nolint:funlen
	tasks []Task,
	startTime, endTime, binDuration float64,
	numDots int,
	filter taskFilter,
	increaseTime, decreaseTime taskTime,
) []TimeValue {
	data := buildEmptyTimeValues(startTime, binDuration, numDots)
	events := make([]countEvent, 0, len(tasks)*2)
	count := 0

	for _, task := range tasks {
		if !filter(task) {
			continue
		}

		start := increaseTime(task)
		end := decreaseTime(task)
		if end <= startTime || start >= endTime || end <= start {
			continue
		}

		if start < startTime {
			count++
		} else {
			events = append(events, countEvent{time: start, delta: 1})
		}

		if end <= endTime {
			events = append(events, countEvent{time: end, delta: -1})
		}
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].time < events[j].time
	})

	eventIndex := 0
	for i := range data {
		binStartTime := float64(i)*binDuration + startTime
		binEndTime := float64(i+1)*binDuration + startTime

		for eventIndex < len(events) && events[eventIndex].time <= binStartTime {
			count += events[eventIndex].delta
			eventIndex++
		}

		timeByCount := 0.0
		prevTime := binStartTime
		for eventIndex < len(events) && events[eventIndex].time < binEndTime {
			eventTime := events[eventIndex].time
			if eventTime > prevTime {
				timeByCount += (eventTime - prevTime) * float64(count)
				prevTime = eventTime
			}

			for eventIndex < len(events) && events[eventIndex].time == eventTime {
				count += events[eventIndex].delta
				eventIndex++
			}
		}

		timeByCount += (binEndTime - prevTime) * float64(count)
		data[i].Value = timeByCount / binDuration
	}

	return data
}

// buildAkitaTraceHeader builds a CSV header from trace data for GPT context.
func buildAkitaTraceHeader(traceReader *SQLiteTraceReader, traceInfo map[string]interface{}) string {
	if traceReader == nil {
		return ""
	}

	selected, _ := traceInfo["selected"].(float64)
	if selected == 0 {
		return ""
	}
	startTime, _ := traceInfo["startTime"].(float64)
	endTime, _ := traceInfo["endTime"].(float64)
	selectedComponentNameList, _ := traceInfo["selectedComponentNameList"].([]interface{})
	locations := extractLocations(selectedComponentNameList)
	if len(locations) == 0 {
		return ""
	}
	sqlStr := buildTraceSQL(locations, startTime, endTime)
	return formatTraceRows(traceReader, sqlStr)
}

func extractLocations(selectedComponentNameList []interface{}) []string {
	locations := make([]string, 0, len(selectedComponentNameList))
	for _, v := range selectedComponentNameList {
		if s, ok := v.(string); ok {
			locations = append(locations, s)
		}
	}
	return locations
}

func buildTraceSQL(locations []string, startTime, endTime float64) string {
	quoted := make([]string, 0, len(locations))
	for _, loc := range locations {
		quoted = append(quoted, "'"+loc+"'")
	}
	// Location is an interned id; join the location table to filter by component
	// name and surface the readable name under the original "Location" column,
	// rather than leaking the integer id (via t.*) or adding an extra column.
	whereClause := "loc.Locale IN (" + strings.Join(quoted, ",") + ")"
	timeClause := fmt.Sprintf(
		"t.StartTime >= %.15f AND t.EndTime <= %.15f", startTime, endTime)
	return `
SELECT
	t.ID,
	t.ParentID,
	t.Kind,
	t.What,
	loc.Locale AS Location,
	t.StartTime,
	t.EndTime
FROM trace t
JOIN location loc ON t.Location = loc.ID
WHERE ` + whereClause + `
AND ` + timeClause
}

func formatTraceRows(traceReader *SQLiteTraceReader, sqlStr string) string {
	rows, err := traceReader.Query(sqlStr)
	if err != nil {
		log.Println("Failed to query trace:", err)
		return ""
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		log.Println("Failed to get columns:", err)
		return ""
	}

	header := "[Reference Akita Trace File]\n" + strings.Join(columns, ",") + "\n"
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			log.Println("Failed to scan trace row:", err)
			continue
		}
		rowStrs := make([]string, 0, len(values))
		for _, val := range values {
			switch v := val.(type) {
			case nil:
				rowStrs = append(rowStrs, "")
			case []byte:
				rowStrs = append(rowStrs, string(v))
			case float64:
				rowStrs = append(rowStrs, fmt.Sprintf("%.9f", v))
			default:
				rowStrs = append(rowStrs, fmt.Sprintf("%v", v))
			}
		}
		header += strings.Join(rowStrs, ",") + "\n"
	}
	header += "[End Akita Trace File]\n"
	return header
}

func buildCombinedRepoHeader(ctx context.Context, urlList []string) string {
	combinedRepoHeader := ""
	for _, url := range urlList {
		content := httpGithubRaw(ctx, url)
		if content == "" {
			continue
		}
		fileName := url
		if idx := strings.Index(url, "sarchlab/"); idx != -1 {
			fileName = url[idx:]
		}
		combinedRepoHeader += "[Reference File " + fileName + "]\n"
		combinedRepoHeader += content + "\n"
		combinedRepoHeader += "[End " + fileName + "]\n"
	}
	return combinedRepoHeader
}

func getRoutineURLList(routineFile string, selectedKeys []string) ([]string, error) {
	data, err := os.ReadFile(routineFile)
	if err != nil {
		return nil, err
	}
	var routineMap map[string][]string
	if err := json.Unmarshal(data, &routineMap); err != nil {
		return nil, err
	}
	urlSet := make(map[string]struct{})
	for _, key := range selectedKeys {
		if urls, ok := routineMap[key]; ok {
			for _, u := range urls {
				urlSet[u] = struct{}{}
			}
		}
	}
	urlList := make([]string, 0, len(urlSet))
	for u := range urlSet {
		urlList = append(urlList, u)
	}
	sort.Strings(urlList)
	return urlList, nil
}

func buildOpenAIPayload(
	ctx context.Context,
	model string,
	messages []map[string]interface{},
	traceInfo map[string]interface{},
	selectedGitHubRoutineKeys []string,
	traceReader *SQLiteTraceReader,
) ([]byte, error) {
	combinedTraceHeader := buildAkitaTraceHeader(traceReader, traceInfo)
	routineFile := "componentgithubroutine.json"
	urlList, err := getRoutineURLList(routineFile, selectedGitHubRoutineKeys)
	if err != nil {
		log.Println("Failed to get routine URL list:", err)
		return nil, err
	}
	combinedRepoHeader := buildCombinedRepoHeader(ctx, urlList)

	if len(messages) > 0 {
		if contentArr, ok := messages[len(messages)-1]["content"].([]interface{}); ok && len(contentArr) > 0 {
			if firstContent, ok := contentArr[0].(map[string]interface{}); ok {
				firstText, _ := firstContent["text"].(string)
				firstContent["text"] = combinedTraceHeader + combinedRepoHeader + firstText
			}
		}
	}

	if len(messages) == 0 || messages[0]["role"] != "system" {
		loadedTextBytes, err := os.ReadFile("beforehandprompt.txt")
		if err != nil {
			log.Println("Failed to read beforehandprompt.txt:", err)
			return nil, err
		}
		loadedText := string(loadedTextBytes)
		systemMsg := map[string]interface{}{
			"role": "system",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": loadedText,
				},
			},
		}
		messages = append([]map[string]interface{}{systemMsg}, messages...)
	}

	payload := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"temperature": 0.7,
	}
	return json.Marshal(payload)
}

func sendOpenAIRequest(ctx context.Context, apiKey, url string, payloadBytes []byte) (*http.Response, error) {
	openaiReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}
	openaiReq.Header.Set("Content-Type", "application/json")
	openaiReq.Header.Set("Authorization", apiKey)
	return http.DefaultClient.Do(openaiReq)
}

func httpGithubRaw(ctx context.Context, url string) string {
	githubPAT := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Println("Failed to create GitHub raw request:", err)
		return ""
	}
	if githubPAT != "" {
		req.Header.Set("Authorization", githubPAT)
	}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		log.Printf("Failed to fetch raw GitHub file: %s, err: %v\n", url, err)
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Failed to read GitHub raw response body:", err)
		return ""
	}
	return string(body)
}

func (s *Server) httpGPTProxy(w http.ResponseWriter, r *http.Request) {
	_ = godotenv.Load(".env")
	openaiApiKey := os.Getenv("OPENAI_API_KEY")
	openaiURL := os.Getenv("OPENAI_URL")
	openaiModel := os.Getenv("OPENAI_MODEL")
	if openaiApiKey == "" || openaiURL == "" || openaiModel == "" {
		http.Error(
			w,
			"[Error: \".env\" not found or OpenAI-related variable missing] "+
				"Please create or update file "+
				"\"akita/daisen/.env\" and write these contents (example):\n"+
				"```\n"+
				"OPENAI_URL=\"https://api.openai.com/v1/chat/completions\"\n"+
				"OPENAI_MODEL=\"gpt-4o\"\n"+
				"OPENAI_API_KEY=\"Bearer sk-proj-XXXXXXXXXXXX\"\n"+
				"GITHUB_PERSONAL_ACCESS_TOKEN=\"Bearer ghp_XXXXXXXXXXXX\"\n"+
				"```\n",
			http.StatusInternalServerError,
		)
		return
	}
	var req struct {
		Messages                  []map[string]interface{} `json:"messages"`
		TraceInfo                 map[string]interface{}   `json:"traceInfo"`
		SelectedGitHubRoutineKeys []string                 `json:"selectedGitHubRoutineKeys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	payloadBytes, err := buildOpenAIPayload(
		r.Context(), openaiModel, req.Messages, req.TraceInfo,
		req.SelectedGitHubRoutineKeys, s.traceReader)
	if err != nil {
		http.Error(w, "Failed to marshal payload: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := sendOpenAIRequest(r.Context(), openaiApiKey, openaiURL, payloadBytes)
	if err != nil {
		http.Error(
			w,
			"Failed to contact OpenAI: "+err.Error(),
			http.StatusBadGateway,
		)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

func (s *Server) httpGithubIsAvailableProxy(w http.ResponseWriter, r *http.Request) {
	_ = godotenv.Load(".env")
	githubPAT := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")
	if githubPAT == "" {
		http.Error(
			w,
			"\n[Error: \".env\" not found or GitHub-related variable missing]\n"+
				"Please create or update file "+
				"\"akita/daisen/.env\" and write these contents (example):\n"+
				"```\n"+
				"OPENAI_URL=\"https://api.openai.com/v1/chat/completions\"\n"+
				"OPENAI_MODEL=\"gpt-4o\"\n"+
				"OPENAI_API_KEY=\"Bearer sk-proj-XXXXXXXXXXXX\"\n"+
				"GITHUB_PERSONAL_ACCESS_TOKEN=\"Bearer ghp_XXXXXXXXXXXX\"\n"+
				"Please refer to "+
				"https://github.com/sarchlab/akita/tree/main/daisen#readme "+
				"for more details.```\n",
			http.StatusInternalServerError,
		)
		return
	}
	client := &http.Client{}
	req, err := http.NewRequestWithContext(r.Context(), "GET", "https://api.github.com/user", nil)
	if err != nil {
		http.Error(w, "Failed to create GitHub request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", githubPAT)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"available":    0,
			"routine_keys": []string{},
		}); err != nil {
			http.Error(w, "Failed to encode JSON: "+err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}
	defer resp.Body.Close()
	routineKeys := []string{}
	routineFile := "componentgithubroutine.json"
	data, err := os.ReadFile(routineFile)
	if err == nil {
		var routineMap map[string]interface{}
		if err := json.Unmarshal(data, &routineMap); err == nil {
			for k := range routineMap {
				routineKeys = append(routineKeys, k)
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"available":    1,
		"routine_keys": routineKeys,
	}); err != nil {
		http.Error(w, "Failed to encode JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// httpCheckEnvFile handles the API endpoint to check if .env file exists
func (s *Server) httpCheckEnvFile(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check if .env file exists
	envFileExists := false
	if _, err := os.Stat(".env"); err == nil {
		envFileExists = true
	}

	// Create response JSON
	response := map[string]interface{}{
		"exists": envFileExists,
	}

	// Encode and send response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (s *Server) fetchTasksForMilestones(ctx context.Context, compName string, startTime, endTime float64) []Task {
	query := TaskQuery{
		Where:            compName,
		EnableTimeRange:  true,
		StartTime:        startTime,
		EndTime:          endTime,
		EnableParentTask: true,
		EnableMilestones: true,
	}
	return s.traceReader.ListTasks(ctx, query)
}

func collectMilestoneKinds(tasks []Task) []string {
	kindSet := make(map[string]bool)
	for _, task := range tasks {
		for _, step := range task.Steps {
			kindSet[step.Kind] = true
		}
	}

	kinds := make([]string, 0, len(kindSet))
	for kind := range kindSet {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

func generateStackedTimeData(tasks []Task, kinds []string, startTime, endTime float64, numDots int) []StackedTimeValue {
	data := make([]StackedTimeValue, 0, numDots)
	totalDuration := endTime - startTime
	binDuration := totalDuration / float64(numDots)

	for i := 0; i < numDots; i++ {
		binStartTime := float64(i)*binDuration + startTime
		binEndTime := float64(i+1)*binDuration + startTime
		kindCounts := countConcurrentTasksByKind(tasks, kinds, binStartTime, binEndTime)

		stv := StackedTimeValue{
			Time:   binStartTime + 0.5*binDuration,
			Values: kindCounts,
		}
		data = append(data, stv)
	}
	return data
}

func countConcurrentTasksByKind(tasks []Task, kinds []string, binStartTime, binEndTime float64) map[string]float64 {
	kindCounts := make(map[string]float64)
	for _, kind := range kinds {
		kindCounts[kind] = 0
	}

	for _, task := range tasks {
		if !isTaskRunningInBin(task, binStartTime, binEndTime) {
			continue
		}

		currentKind := findTaskMilestoneKind(task, binStartTime)
		if currentKind != "" {
			kindCounts[currentKind]++
		}
	}
	return kindCounts
}

func isTaskRunningInBin(task Task, binStartTime, binEndTime float64) bool {
	return !(float64(task.EndTime) < binStartTime || float64(task.StartTime) > binEndTime)
}

func findTaskMilestoneKind(task Task, binStartTime float64) string {
	var currentKind string
	var latestTime float64 = -1

	// Find the most recent milestone before or at the bin start time
	for _, step := range task.Steps {
		stepTime := float64(step.Time)
		if stepTime <= binStartTime && stepTime > latestTime {
			latestTime = stepTime
			currentKind = step.Kind
		}
	}

	// If no milestone found before this bin, use the first milestone of the task
	if currentKind == "" && len(task.Steps) > 0 {
		firstStep := task.Steps[0]
		for _, step := range task.Steps {
			if float64(step.Time) < float64(firstStep.Time) {
				firstStep = step
			}
		}
		currentKind = firstStep.Kind
	}

	return currentKind
}

func (s *Server) calculateConcurrentTaskMilestones(
	ctx context.Context,
	compName, infoType string,
	startTime, endTime float64,
	numDots int,
) *StackedComponentInfo {
	info := &StackedComponentInfo{
		Name:      compName,
		InfoType:  infoType,
		StartTime: startTime,
		EndTime:   endTime,
		Kinds:     []string{},
	}

	tasks := s.fetchTasksForMilestones(ctx, compName, startTime, endTime)
	info.Kinds = collectMilestoneKinds(tasks)
	info.Data = generateStackedTimeData(tasks, info.Kinds, startTime, endTime, numDots)

	return info
}
