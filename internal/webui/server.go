package webui

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ftdcstat/internal/derive"
	"ftdcstat/internal/model"
	"ftdcstat/internal/render"
)

//go:embed static/index.html
//go:embed static/*
var assets embed.FS

type Options struct {
	View         string
	Avg          time.Duration
	TimeRange    model.TimeRange
	TimeLocation *time.Location
}

type Dataset struct {
	Metadata MetadataResponse `json:"metadata"`
	Data     DataResponse     `json:"data"`
}

type MetadataResponse struct {
	View       string          `json:"view"`
	Avg        AvgInfo         `json:"avg"`
	TimeRange  TimeRangeInfo   `json:"timeRange"`
	HeaderText string          `json:"headerText"`
	Metadata   map[string]any  `json:"metadata"`
	Warnings   []model.Warning `json:"warnings,omitempty"`
	Sections   []Section       `json:"sections"`
	RowCount   int             `json:"rowCount"`
}

type DataResponse struct {
	View     string              `json:"view"`
	Avg      AvgInfo             `json:"avg"`
	Sections map[string][]string `json:"sections"`
	Rows     []DataRow           `json:"rows"`
}

type AvgInfo struct {
	Enabled bool   `json:"enabled"`
	Bucket  string `json:"bucket,omitempty"`
}

type TimeRangeInfo struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type HeaderSection struct {
	Name  string       `json:"name"`
	Items []HeaderItem `json:"items"`
}

type HeaderItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Section struct {
	Name    string   `json:"name"`
	Metrics []Metric `json:"metrics"`
}

type Metric struct {
	Column   string `json:"column"`
	Label    string `json:"label"`
	JSONName string `json:"jsonName"`
	Format   string `json:"format"`
	Default  bool   `json:"default"`
}

type DataRow struct {
	Datetime string                    `json:"datetime"`
	Sections map[string]map[string]any `json:"-"`
	Values   map[string]map[string]any `json:"-"`
}

type Server struct {
	dataset    Dataset
	indexHTML  []byte
	appJS      []byte
	styleCSS   []byte
	metaJSON   []byte
	dataJSON   []byte
	listenerFD int
	host       string
	port       int
}

func ResolveListenAddress(listen string) string {
	if strings.TrimSpace(listen) == "" {
		return "127.0.0.1:0"
	}
	return listen
}

func BuildDataset(metadata model.Metadata, warnings []model.Warning, rows []derive.Row, renderOpts render.Options, opts Options) Dataset {
	loc := opts.TimeLocation
	if loc == nil {
		loc = time.UTC
	}
	if opts.Avg > 0 {
		rows = averageRows(rows, opts.Avg)
	}
	desc := render.DescribeView(metadata, rows, renderOpts)
	sections := buildSections(desc, opts.View)
	return Dataset{
		Metadata: MetadataResponse{
			View:       opts.View,
			Avg:        avgInfo(opts.Avg),
			TimeRange:  timeRangeInfo(opts.TimeRange, loc),
			HeaderText: render.HeaderText(metadata, loc),
			Metadata:   metadata.Summary(),
			Warnings:   append([]model.Warning(nil), warnings...),
			Sections:   sections,
			RowCount:   len(rows),
		},
		Data: DataResponse{
			View:     opts.View,
			Avg:      avgInfo(opts.Avg),
			Sections: sectionColumns(sections),
			Rows:     buildRows(rows, sections, loc),
		},
	}
}

func NewServer(dataset Dataset) (*Server, error) {
	indexBytes, err := assets.ReadFile("static/index.html")
	if err != nil {
		return nil, err
	}
	appJS, err := assets.ReadFile("static/app.js")
	if err != nil {
		return nil, err
	}
	styleCSS, err := assets.ReadFile("static/style.css")
	if err != nil {
		return nil, err
	}
	indexHTML := bytes.ReplaceAll(indexBytes, []byte("{{ .Title }}"), []byte(fmt.Sprintf("ftdcstat web UI - %s", dataset.Metadata.View)))
	metaJSON, err := marshalJSON(dataset.Metadata)
	if err != nil {
		return nil, err
	}
	dataJSON, err := marshalJSON(dataset.Data)
	if err != nil {
		return nil, err
	}
	return &Server{
		dataset:   dataset,
		indexHTML: indexHTML,
		appJS:     appJS,
		styleCSS:  styleCSS,
		metaJSON:  metaJSON,
		dataJSON:  dataJSON,
	}, nil
}

func (s *Server) ListenAndServe(listen string) (string, error) {
	host, port, err := parseListenAddress(ResolveListenAddress(listen))
	if err != nil {
		return "", err
	}
	fd, actualPort, err := listenTCP4(host, port)
	if err != nil {
		return "", err
	}
	s.listenerFD = fd
	s.host = host
	s.port = actualPort
	address := fmt.Sprintf("http://%s:%d", host, actualPort)
	return address, s.serveLoop()
}

func (s *Server) Listen(listen string) (string, error) {
	host, port, err := parseListenAddress(ResolveListenAddress(listen))
	if err != nil {
		return "", err
	}
	fd, actualPort, err := listenTCP4(host, port)
	if err != nil {
		return "", err
	}
	s.listenerFD = fd
	s.host = host
	s.port = actualPort
	return fmt.Sprintf("http://%s:%d", host, actualPort), nil
}

func (s *Server) Serve() error {
	return s.serveLoop()
}

func (s *Server) serveLoop() error {
	if s.listenerFD == 0 {
		return fmt.Errorf("server is not listening")
	}
	for {
		connFD, _, err := syscall.Accept(s.listenerFD)
		if err != nil {
			if err == syscall.EINVAL || err == syscall.EBADF {
				return nil
			}
			continue
		}
		go s.serveConn(connFD)
	}
}

func (s *Server) Close() error {
	if s.listenerFD == 0 {
		return nil
	}
	err := syscall.Close(s.listenerFD)
	s.listenerFD = 0
	return err
}

func (s *Server) serveConn(fd int) {
	file := os.NewFile(uintptr(fd), "webui-conn")
	if file == nil {
		_ = syscall.Close(fd)
		return
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	method, path, err := readRequestLine(reader)
	if err != nil {
		_, _ = file.Write(httpResponse(400, "text/plain; charset=utf-8", []byte("bad request\n")))
		return
	}
	if err := discardHeaders(reader); err != nil {
		_, _ = file.Write(httpResponse(400, "text/plain; charset=utf-8", []byte("bad request\n")))
		return
	}
	if method != "GET" {
		_, _ = file.Write(httpResponse(405, "text/plain; charset=utf-8", []byte("method not allowed\n")))
		return
	}
	body, contentType, status := s.route(path)
	_, _ = file.Write(httpResponse(status, contentType, body))
}

func (s *Server) route(path string) ([]byte, string, int) {
	switch path {
	case "/":
		return s.indexHTML, "text/html; charset=utf-8", 200
	case "/app.js":
		return s.appJS, "application/javascript; charset=utf-8", 200
	case "/style.css":
		return s.styleCSS, "text/css; charset=utf-8", 200
	case "/api/metadata":
		return s.metaJSON, "application/json; charset=utf-8", 200
	case "/api/data":
		return s.dataJSON, "application/json; charset=utf-8", 200
	default:
		return []byte("not found\n"), "text/plain; charset=utf-8", 404
	}
}

func marshalJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func httpResponse(status int, contentType string, body []byte) []byte {
	statusText := "OK"
	switch status {
	case 200:
		statusText = "OK"
	case 400:
		statusText = "Bad Request"
	case 404:
		statusText = "Not Found"
	case 405:
		statusText = "Method Not Allowed"
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "HTTP/1.1 %d %s\r\n", status, statusText)
	fmt.Fprintf(&buf, "Content-Type: %s\r\n", contentType)
	fmt.Fprintf(&buf, "Content-Length: %d\r\n", len(body))
	fmt.Fprintf(&buf, "Connection: close\r\n\r\n")
	buf.Write(body)
	return buf.Bytes()
}

func readRequestLine(reader *bufio.Reader) (string, string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", "", err
	}
	line = strings.TrimSpace(line)
	parts := strings.Split(line, " ")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid request line")
	}
	return parts[0], parts[1], nil
}

func discardHeaders(reader *bufio.Reader) error {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if line == "\r\n" || line == "\n" {
			return nil
		}
	}
}

func parseListenAddress(value string) (string, int, error) {
	lastColon := strings.LastIndex(value, ":")
	if lastColon <= 0 || lastColon == len(value)-1 {
		return "", 0, fmt.Errorf("listen address must be host:port")
	}
	host := value[:lastColon]
	portText := value[lastColon+1:]
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return "", 0, fmt.Errorf("listen port must be between 0 and 65535")
	}
	switch host {
	case "localhost":
		host = "127.0.0.1"
	case "":
		host = "0.0.0.0"
	}
	if _, err := parseIPv4(host); err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func parseIPv4(host string) ([4]byte, error) {
	var out [4]byte
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return out, fmt.Errorf("listen host must be an IPv4 address or localhost")
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 255 {
			return out, fmt.Errorf("listen host must be an IPv4 address or localhost")
		}
		out[i] = byte(n)
	}
	return out, nil
}

func listenTCP4(host string, port int) (int, int, error) {
	addrBytes, err := parseIPv4(host)
	if err != nil {
		return 0, 0, err
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return 0, 0, err
	}
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		_ = syscall.Close(fd)
		return 0, 0, err
	}
	addr := &syscall.SockaddrInet4{Port: port, Addr: addrBytes}
	if err := syscall.Bind(fd, addr); err != nil {
		_ = syscall.Close(fd)
		return 0, 0, err
	}
	if err := syscall.Listen(fd, 16); err != nil {
		_ = syscall.Close(fd)
		return 0, 0, err
	}
	actual, err := syscall.Getsockname(fd)
	if err != nil {
		_ = syscall.Close(fd)
		return 0, 0, err
	}
	inet, ok := actual.(*syscall.SockaddrInet4)
	if !ok {
		_ = syscall.Close(fd)
		return 0, 0, fmt.Errorf("unexpected socket type")
	}
	return fd, inet.Port, nil
}

func avgInfo(bucket time.Duration) AvgInfo {
	if bucket <= 0 {
		return AvgInfo{}
	}
	return AvgInfo{Enabled: true, Bucket: bucket.String()}
}

func timeRangeInfo(r model.TimeRange, loc *time.Location) TimeRangeInfo {
	var out TimeRangeInfo
	if !r.From.IsZero() {
		out.From = r.From.In(loc).Format(time.RFC3339)
	}
	if !r.To.IsZero() {
		out.To = r.To.In(loc).Format(time.RFC3339)
	}
	return out
}

func buildSections(desc render.ViewDescription, view string) []Section {
	if hasDashboardSplitCandidates(desc.Sections) {
		return buildDashboardSections(desc, view)
	}
	return buildDefaultSections(desc.Sections, view)
}

func buildDefaultSections(descSections []render.ViewSection, view string) []Section {
	out := make([]Section, 0, len(descSections))
	for _, section := range descSections {
		metrics := make([]Metric, 0, len(section.Columns))
		for _, column := range section.Columns {
			if column == "lagSLabel" {
				continue
			}
			info := render.MetricInfoForColumn(column)
			metrics = append(metrics, Metric{
				Column:   column,
				Label:    column,
				JSONName: info.JSONName,
				Format:   info.Format,
				Default:  defaultMetricForView(view, section.Name, column),
			})
		}
		out = append(out, Section{Name: section.Name, Metrics: metrics})
	}
	return out
}

func buildDashboardSections(desc render.ViewDescription, view string) []Section {
	sectionColumns := map[string]map[string]bool{}
	for _, section := range desc.Sections {
		cols := sectionColumns[section.Name]
		if cols == nil {
			cols = map[string]bool{}
			sectionColumns[section.Name] = cols
		}
		for _, column := range section.Columns {
			cols[column] = true
		}
	}

	defs := []struct {
		name         string
		source       string
		defaultInAll bool
		columns      []string
	}{
		{name: "server / Commands", source: "server", defaultInAll: false, columns: []string{"qTot", "ins/s", "qry/s", "upd/s", "del/s", "getm/s", "cmd/s"}},
		{name: "server / Latency", source: "server", defaultInAll: false, columns: []string{"rLatS", "wLatS", "cLatS"}},
		{name: "system / CPU", source: "system", defaultInAll: false, columns: []string{"user_cpu%", "system_cpu%", "iowait%", "ctxt/s"}},
		{name: "system / Memory", source: "system", defaultInAll: false, columns: []string{"residentMB", "virtualMB", "swapIn/s", "swapOut/s"}},
		{name: "system / Disks", source: "system", defaultInAll: false, columns: []string{"r/s", "w/s", "rkB/s", "wkB/s", "awaitS", "r_awaitS", "w_awaitS", "aqu-sz", "util%"}},
		{name: "system / PSI", source: "pressure", defaultInAll: true, columns: []string{"psiCpuSome%", "psiMemSome%", "psiMemFull%", "psiIoSome%", "psiIoFull%"}},
		{name: "wiredTiger / Tickets", source: "wiredTiger", defaultInAll: false, columns: []string{"rdTkt", "wrTkt"}},
		{name: "wiredTiger / Per-second rates", source: "wiredTiger", defaultInAll: false, columns: []string{"wtRdMB/s", "wtWrMB/s", "evict/s", "appEvict/s", "evictWalks/s", "evictBusy/s", "ckptPages/s", "hsInsert/s", "hsRead/s", "hsWriteMB/s"}},
		{name: "wiredTiger / Checkpoint time", source: "wiredTiger", defaultInAll: false, columns: []string{"ckptMS"}},
		{name: "wiredTiger / Percentages", source: "wiredTiger", defaultInAll: false, columns: []string{"wtCache%", "dirty%"}},
		{name: "wiredTiger / MiB", source: "wiredTiger", defaultInAll: false, columns: []string{"cacheMB", "dirtyMB", "updatesMB"}},
	}

	usedColumns := map[string]map[string]bool{}
	var splitOut []Section
	appendSplitSections := func() {
		if len(splitOut) > 0 {
			return
		}
		for _, def := range defs {
			sourceCols := sectionColumns[def.source]
			if len(sourceCols) == 0 {
				continue
			}
			metrics := make([]Metric, 0, len(def.columns))
			for _, column := range def.columns {
				if !sourceCols[column] {
					continue
				}
				info := render.MetricInfoForColumn(column)
				metrics = append(metrics, Metric{
					Column:   column,
					Label:    column,
					JSONName: info.JSONName,
					Format:   info.Format,
					Default:  def.defaultInAll || defaultMetricForView(view, def.name, column),
				})
				if usedColumns[def.source] == nil {
					usedColumns[def.source] = map[string]bool{}
				}
				usedColumns[def.source][column] = true
			}
			if len(metrics) > 0 {
				splitOut = append(splitOut, Section{Name: def.name, Metrics: metrics})
			}
		}
	}

	var out []Section
	for _, section := range desc.Sections {
		if section.Name == "server" || section.Name == "system" || section.Name == "pressure" || section.Name == "wiredTiger" {
			if section.Name == "server" || section.Name == "system" || section.Name == "wiredTiger" {
				appendSplitSections()
				for _, split := range splitOut {
					if strings.HasPrefix(split.Name, section.Name+" /") {
						out = append(out, split)
					}
				}
			}
			sourceCols := usedColumns[section.Name]
			var remaining []string
			for _, column := range section.Columns {
				if column == "lagSLabel" {
					continue
				}
				if sourceCols != nil && sourceCols[column] {
					continue
				}
				remaining = append(remaining, column)
			}
			if len(remaining) == 0 {
				continue
			}
			out = append(out, buildDefaultSections([]render.ViewSection{{Name: section.Name, Columns: remaining}}, view)...)
			continue
		}
		out = append(out, buildDefaultSections([]render.ViewSection{section}, view)...)
	}
	return out
}

func hasDashboardSplitCandidates(sections []render.ViewSection) bool {
	for _, section := range sections {
		if section.Name == "server" || section.Name == "system" || section.Name == "pressure" || section.Name == "wiredTiger" {
			return true
		}
	}
	return false
}

func defaultMetricForView(view, section, column string) bool {
	if view != "summary" {
		return true
	}
	switch section {
	case "replication":
		return column == "majLagS" || strings.HasPrefix(column, "node")
	case "server":
		return inSet(column, "qTot", "rLatS", "wLatS", "cLatS")
	case "server / Commands":
		return column == "qTot"
	case "server / Latency":
		return inSet(column, "rLatS", "wLatS", "cLatS")
	case "network":
		return inSet(column, "activeConn", "totalCreated/s", "queuedConn", "rejConn/s")
	case "system", "system / CPU":
		return inSet(column, "iowait%", "user_cpu%", "system_cpu%")
	case "system / Memory":
		return inSet(column, "residentMB")
	case "system / Disks":
		return inSet(column, "awaitS", "util%")
	case "system / PSI":
		return false
	case "wiredTiger":
		return inSet(column, "wtCache%", "dirty%", "evict/s", "appEvict/s", "ckptMS", "rdTkt", "wrTkt")
	case "wiredTiger / Tickets":
		return inSet(column, "rdTkt", "wrTkt")
	case "wiredTiger / Per-second rates":
		return inSet(column, "evict/s", "appEvict/s")
	case "wiredTiger / Checkpoint time":
		return column == "ckptMS"
	case "wiredTiger / Percentages":
		return inSet(column, "wtCache%", "dirty%")
	case "wiredTiger / MiB":
		return false
	default:
		return false
	}
}

func inSet(value string, set ...string) bool {
	for _, item := range set {
		if value == item {
			return true
		}
	}
	return false
}

func buildRows(rows []derive.Row, sections []Section, loc *time.Location) []DataRow {
	out := make([]DataRow, 0, len(rows))
	for _, row := range rows {
		item := DataRow{
			Datetime: row.Time.In(loc).Format(time.RFC3339),
			Sections: map[string]map[string]any{},
		}
		for _, section := range sections {
			values := map[string]any{}
			for _, metric := range section.Metrics {
				values[metric.JSONName] = nil
				if value, ok := row.Values[metric.Column]; ok {
					values[metric.JSONName] = value
				}
			}
			item.Sections[section.Name] = values
		}
		item.Values = item.Sections
		out = append(out, item)
	}
	return out
}

func (r DataRow) MarshalJSON() ([]byte, error) {
	item := map[string]any{"datetime": r.Datetime}
	for key, value := range r.Sections {
		item[key] = value
	}
	return json.Marshal(item)
}

func (r *DataRow) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Sections = map[string]map[string]any{}
	for key, value := range raw {
		if key == "datetime" {
			if err := json.Unmarshal(value, &r.Datetime); err != nil {
				return err
			}
			continue
		}
		var section map[string]any
		if err := json.Unmarshal(value, &section); err != nil {
			return err
		}
		r.Sections[key] = section
	}
	r.Values = r.Sections
	return nil
}

func sectionColumns(sections []Section) map[string][]string {
	out := make(map[string][]string, len(sections))
	for _, section := range sections {
		cols := make([]string, 0, len(section.Metrics))
		for _, metric := range section.Metrics {
			cols = append(cols, metric.JSONName)
		}
		out[section.Name] = cols
	}
	return out
}

func averageRows(rows []derive.Row, bucket time.Duration) []derive.Row {
	if bucket <= 0 || len(rows) == 0 {
		return append([]derive.Row(nil), rows...)
	}
	type aggregate struct {
		start   time.Time
		sums    map[string]float64
		counts  map[string]float64
		strings map[string]any
		marker  string
		process string
	}
	var out []derive.Row
	var cur aggregate
	flush := func() {
		if cur.start.IsZero() {
			return
		}
		values := make(map[string]any, len(cur.sums)+len(cur.strings))
		for key, sum := range cur.sums {
			if count := cur.counts[key]; count > 0 {
				values[key] = sum / count
			}
		}
		for key, value := range cur.strings {
			if _, exists := values[key]; !exists {
				values[key] = value
			}
		}
		out = append(out, derive.Row{
			Time:          cur.start,
			Marker:        cur.marker,
			ProcessMarker: cur.process,
			Values:        values,
		})
	}
	reset := func(start time.Time) {
		cur = aggregate{
			start:   start,
			sums:    map[string]float64{},
			counts:  map[string]float64{},
			strings: map[string]any{},
		}
	}
	for _, row := range rows {
		start := row.Time.Truncate(bucket)
		if cur.start.IsZero() {
			reset(start)
		} else if !start.Equal(cur.start) {
			flush()
			reset(start)
		}
		if cur.marker == "" && row.Marker != "" {
			cur.marker = row.Marker
		}
		if cur.process == "" && row.ProcessMarker != "" {
			cur.process = row.ProcessMarker
		}
		for key, value := range row.Values {
			if number, ok := model.AsFloat(value); ok {
				cur.sums[key] += number
				cur.counts[key]++
				continue
			}
			cur.strings[key] = value
		}
	}
	flush()
	return out
}

func MetricNames(section Section) []string {
	names := make([]string, 0, len(section.Metrics))
	for _, metric := range section.Metrics {
		names = append(names, metric.Column)
	}
	sort.Strings(names)
	return names
}
