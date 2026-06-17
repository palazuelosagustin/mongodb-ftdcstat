package derive

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"ftdcstat/internal/model"
)

type Options struct {
	IntervalSeconds int
	GapThreshold    time.Duration
	Device          string
	Metadata        model.Metadata
	TimeLocation    *time.Location
}

type Row struct {
	Time          time.Time      `json:"time"`
	Kind          string         `json:"kind,omitempty"`
	Marker        string         `json:"marker,omitempty"`
	ProcessMarker string         `json:"processMarker,omitempty"`
	Values        map[string]any `json:"values"`
}

type ReplMember struct {
	Label string `json:"label"`
	Name  string `json:"name"`
}

type ReplSetInfo struct {
	Set     string       `json:"set"`
	Members []ReplMember `json:"members"`
}

type Streamer struct {
	opts           Options
	lastRendered   time.Time
	printedProcess bool
	replMembers    *replMemberRegistry
	prev           model.MetricSample
	havePrev       bool
}

func Rows(samples []model.MetricSample, opts Options) []Row {
	streamer := NewStreamer(opts)
	var rows []Row
	for _, sample := range samples {
		if row, ok := streamer.Add(sample); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func NewStreamer(opts Options) *Streamer {
	if opts.IntervalSeconds <= 0 {
		opts.IntervalSeconds = 60
	}
	if opts.GapThreshold <= 0 {
		opts.GapThreshold = time.Duration(maxInt(60, opts.IntervalSeconds*10)) * time.Second
	}
	return &Streamer{
		opts:        opts,
		replMembers: newReplMemberRegistry(opts.Metadata),
	}
}

func (s *Streamer) Add(cur model.MetricSample) (Row, bool) {
	if !s.havePrev {
		if cur.Time.IsZero() {
			return Row{}, false
		}
		s.prev = cur
		s.havePrev = true
		return Row{}, false
	}
	prev := s.prev
	s.prev = cur
	if cur.Time.IsZero() || prev.Time.IsZero() || !cur.Time.After(prev.Time) {
		return Row{}, false
	}
	if !s.lastRendered.IsZero() && cur.Time.Sub(s.lastRendered) < time.Duration(s.opts.IntervalSeconds)*time.Second {
		return Row{}, false
	}
	calc := calculator{prev: prev, cur: cur, dt: cur.Time.Sub(prev.Time).Seconds()}
	row := Row{Time: cur.Time, Values: map[string]any{}}
	restarted := processRestart(prev, cur)
	if cur.Time.Sub(prev.Time) > s.opts.GapThreshold {
		row.Marker = fmt.Sprintf("gap %.0fs: rate baseline reset", cur.Time.Sub(prev.Time).Seconds())
	}
	if !s.printedProcess {
		row.ProcessMarker = processMarker("process", cur, s.opts.TimeLocation)
		s.printedProcess = row.ProcessMarker != ""
	}
	if restarted {
		row.ProcessMarker = processMarker("restart detected", cur, s.opts.TimeLocation)
	}
	reset := row.Marker != "" || restarted
	fillSummary(&row, calc, reset)
	fillWT(&row, calc, reset)
	fillMemoryLatencyConnections(&row, calc, reset)
	fillNetwork(&row, calc, reset)
	fillCPU(&row, calc, s.opts, reset)
	fillDisk(&row, calc, s.opts.Device, reset)
	fillReplication(&row, calc, s.replMembers, reset)
	s.lastRendered = cur.Time
	return row, true
}

func MergeSamples(samples []model.MetricSample, warnings *[]model.Warning) []model.MetricSample {
	sort.SliceStable(samples, func(i, j int) bool {
		if samples[i].Time.Equal(samples[j].Time) {
			return samples[i].SourceIndex < samples[j].SourceIndex
		}
		return samples[i].Time.Before(samples[j].Time)
	})
	out := samples[:0]
	for _, sample := range samples {
		if sample.Time.IsZero() {
			if warnings != nil {
				*warnings = append(*warnings, model.Warning{Source: sample.Source, Message: "sample without timestamp skipped"})
			}
			continue
		}
		if len(out) > 0 && sample.Time.Equal(out[len(out)-1].Time) {
			out[len(out)-1] = sample
			continue
		}
		out = append(out, sample)
	}
	return out
}

type calculator struct {
	prev model.MetricSample
	cur  model.MetricSample
	dt   float64
}

func (c calculator) current(path string) (float64, bool) {
	return c.cur.Get(path)
}

func (c calculator) currentAny(paths ...string) (float64, bool) {
	return c.cur.GetAny(paths...)
}

func (c calculator) rate(path string) (float64, bool) {
	prev, ok := c.prev.Get(path)
	if !ok {
		return 0, false
	}
	cur, ok := c.cur.Get(path)
	if !ok || c.dt <= 0 || cur < prev {
		return 0, false
	}
	return (cur - prev) / c.dt, true
}

func (c calculator) rateAny(paths ...string) (float64, bool) {
	for _, path := range paths {
		if v, ok := c.rate(path); ok {
			return v, true
		}
	}
	return 0, false
}

func (c calculator) delta(path string) (float64, bool) {
	prev, ok := c.prev.Get(path)
	if !ok {
		return 0, false
	}
	cur, ok := c.cur.Get(path)
	if !ok || cur < prev {
		return 0, false
	}
	return cur - prev, true
}

func fillSummary(row *Row, c calculator, reset bool) {
	setCurrent(row, "conn", c, "serverStatus.connections.current")
	setCurrent(row, "qTot", c, "serverStatus.globalLock.currentQueue.total")
	row.Values["rsState"] = rsState(c.cur)
	if !reset {
		setRate(row, "ins/s", c, "serverStatus.opcounters.insert")
		setRate(row, "qry/s", c, "serverStatus.opcounters.query")
		setRate(row, "upd/s", c, "serverStatus.opcounters.update")
		setRate(row, "del/s", c, "serverStatus.opcounters.delete")
		setRate(row, "getm/s", c, "serverStatus.opcounters.getmore")
		setRate(row, "cmd/s", c, "serverStatus.opcounters.command")
		setLatency(row, "rLatS", c, "serverStatus.opLatencies.reads")
		setLatency(row, "wLatS", c, "serverStatus.opLatencies.writes")
		setLatency(row, "cLatS", c, "serverStatus.opLatencies.commands")
	}
}

func fillWT(row *Row, c calculator, reset bool) {
	curCache, curOK := c.current("serverStatus.wiredTiger.cache.bytes currently in the cache")
	maxCache, maxOK := c.current("serverStatus.wiredTiger.cache.maximum bytes configured")
	dirty, dirtyOK := c.current("serverStatus.wiredTiger.cache.tracked dirty bytes in the cache")
	if curOK && maxOK && maxCache > 0 {
		row.Values["wtCache%"] = curCache / maxCache * 100
	}
	if dirtyOK && maxOK && maxCache > 0 {
		row.Values["dirty%"] = dirty / maxCache * 100
	}
	if curOK {
		row.Values["cacheMB"] = curCache / 1024 / 1024
	}
	if dirtyOK {
		row.Values["dirtyMB"] = dirty / 1024 / 1024
	}
	if v, ok := c.currentAny(
		"serverStatus.wiredTiger.cache.bytes belonging to the updates in the cache",
		"serverStatus.wiredTiger.cache.tracked bytes belonging to the updates in the cache",
		"serverStatus.wiredTiger.cache.bytes allocated for updates",
	); ok {
		row.Values["updatesMB"] = v / 1024 / 1024
	}
	if !reset {
		if v, ok := c.rate("serverStatus.wiredTiger.cache.bytes read into cache"); ok {
			row.Values["wtRdMB/s"] = v / 1024 / 1024
		}
		if v, ok := c.rate("serverStatus.wiredTiger.cache.bytes written from cache"); ok {
			row.Values["wtWrMB/s"] = v / 1024 / 1024
		}
		setRateSum(row, "evict/s", c,
			"serverStatus.wiredTiger.cache.pages evicted by eviction server",
			"serverStatus.wiredTiger.cache.pages evicted by application threads",
		)
		if _, ok := row.Values["evict/s"]; !ok {
			setRateSum(row, "evict/s", c,
				"serverStatus.wiredTiger.cache.pages evicted because they exceeded the in-memory maximum",
				"serverStatus.wiredTiger.cache.unmodified pages evicted",
				"serverStatus.wiredTiger.cache.modified pages evicted",
			)
		}
		if _, ok := row.Values["evict/s"]; !ok {
			setRateSum(row, "evict/s", c,
				"serverStatus.wiredTiger.cache.eviction server candidate queue empty when topping up",
				"serverStatus.wiredTiger.cache.eviction server candidate queue not empty when topping up",
			)
		}
		setRate(row, "appEvict/s", c, "serverStatus.wiredTiger.cache.application threads page read from disk to cache count")
		setRateSum(row, "evictWalks/s", c,
			"serverStatus.wiredTiger.cache.eviction walks started from root of tree",
			"serverStatus.wiredTiger.cache.eviction walks started from saved location in tree",
		)
		setRateSum(row, "evictBusy/s", c,
			"serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted",
			"serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted because of active children on an internal page",
			"serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted because of failure in reconciliation",
			"serverStatus.wiredTiger.cache.pages selected for eviction unable to be evicted because of a cache overflow item",
		)
		setRateAny(row, "ckptPages/s", c,
			"serverStatus.wiredTiger.transaction.transaction checkpoint pages written",
			"serverStatus.wiredTiger.checkpoint-cleanup.pages written",
			"serverStatus.wiredTiger.checkpoint.number of pages caused to be reconciled",
		)
		setRateAny(row, "hsInsert/s", c,
			"serverStatus.wiredTiger.cache.history store table insert calls",
			"serverStatus.wiredTiger.history store.history store table insert calls",
		)
		setRateAny(row, "hsRead/s", c,
			"serverStatus.wiredTiger.cache.history store table read calls",
			"serverStatus.wiredTiger.cache.history store table reads",
			"serverStatus.wiredTiger.history store.history store table read calls",
		)
		setRateScaledAny(row, "hsWriteMB/s", c, 1024*1024,
			"serverStatus.wiredTiger.cache.bytes written from cache into history store",
			"serverStatus.wiredTiger.history store.history store table bytes written",
		)
	}
	if v, ok := c.currentAny(
		"serverStatus.wiredTiger.transaction.transaction checkpoint most recent duration for gathering all handles (usecs)",
		"serverStatus.wiredTiger.checkpoint.most recent duration for gathering all handles (usecs)",
	); ok {
		row.Values["ckptMS"] = v / 1000
	}
	if v, ok := c.currentAny("serverStatus.wiredTiger.concurrentTransactions.read.available", "serverStatus.queues.execution.read.available"); ok {
		row.Values["rdTkt"] = v
	}
	if v, ok := c.currentAny("serverStatus.wiredTiger.concurrentTransactions.write.available", "serverStatus.queues.execution.write.available"); ok {
		row.Values["wrTkt"] = v
	}
}

func fillMemoryLatencyConnections(row *Row, c calculator, reset bool) {
	setCurrent(row, "residentMB", c, "serverStatus.mem.resident")
	setCurrent(row, "virtualMB", c, "serverStatus.mem.virtual")
	_ = reset
}

func fillNetwork(row *Row, c calculator, reset bool) {
	setCurrent(row, "activeConn", c, "serverStatus.connections.active")
	if current, ok := c.current("serverStatus.connections.current"); ok {
		if active, ok := c.current("serverStatus.connections.active"); ok {
			idle := current - active
			if idle < 0 {
				idle = 0
			}
			row.Values["idleConn"] = idle
		}
	}
	setCurrent(row, "queuedConn", c, "serverStatus.connections.queuedForEstablishment")
	if reset {
		return
	}
	setRate(row, "totalCreated/s", c, "serverStatus.connections.totalCreated")
	setRate(row, "rejConn/s", c, "serverStatus.connections.rejected")
	setRate(row, "dnsSlow/s", c, "serverStatus.network.numSlowDNSOperations")
	setRate(row, "tlsSlow/s", c, "serverStatus.network.numSlowSSLOperations")
	setRate(row, "netTimeout/s", c, "serverStatus.metrics.operation.numConnectionNetworkTimeouts")
}

func fillCPU(row *Row, c calculator, opts Options, reset bool) {
	setCurrent(row, "r", c, "systemMetrics.cpu.procs_running")
	setCurrent(row, "b", c, "systemMetrics.cpu.procs_blocked")
	if reset {
		return
	}
	cpu := map[string]float64{}
	var total float64
	for _, name := range []string{"user_ms", "nice_ms", "system_ms", "idle_ms", "iowait_ms", "irq_ms", "softirq_ms", "steal_ms"} {
		if delta, ok := c.delta("systemMetrics.cpu." + name); ok {
			cpu[name] = delta
			total += delta
		}
	}
	if total > 0 {
		row.Values["iowait%"] = cpu["iowait_ms"] / total * 100
		row.Values["steal%"] = cpu["steal_ms"] / total * 100
		row.Values["idle%"] = cpu["idle_ms"] / total * 100
		row.Values["cpu%"] = 100 - cpu["idle_ms"]/total*100
	}
	cpuCount := cpuCountFromMetadata(opts.Metadata)
	if cpuCount <= 0 {
		cpuCount = 1
	}
	if userDelta, ok := c.delta("serverStatus.extra_info.user_time_us"); ok && c.dt > 0 {
		row.Values["user_cpu%"] = userDelta / (c.dt * 1_000_000 * cpuCount) * 100
	} else if total > 0 {
		row.Values["user_cpu%"] = (cpu["user_ms"] + cpu["nice_ms"]) / total * 100
	}
	if systemDelta, ok := c.delta("serverStatus.extra_info.system_time_us"); ok && c.dt > 0 {
		row.Values["system_cpu%"] = systemDelta / (c.dt * 1_000_000 * cpuCount) * 100
	} else if total > 0 {
		row.Values["system_cpu%"] = (cpu["system_ms"] + cpu["irq_ms"] + cpu["softirq_ms"]) / total * 100
	}
	setRate(row, "swapIn/s", c, "systemMetrics.vmstat.pswpin")
	setRate(row, "swapOut/s", c, "systemMetrics.vmstat.pswpout")
	setRate(row, "ctxt/s", c, "systemMetrics.cpu.ctxt")
	setPressurePercent(row, "psiCpuSome%", c, "cpu", "some")
	setPressurePercent(row, "psiMemSome%", c, "memory", "some")
	setPressurePercent(row, "psiMemFull%", c, "memory", "full")
	setPressurePercent(row, "psiIoSome%", c, "io", "some")
	setPressurePercent(row, "psiIoFull%", c, "io", "full")
}

func fillDisk(row *Row, c calculator, device string, reset bool) {
	devices := diskDevices(c.cur, device)
	sort.Strings(devices)
	var totalReadKB, totalWriteKB, maxUtil float64
	var totalReadOps, totalWriteOps, totalReadTimeMS, totalWriteTimeMS, totalQueuedMS float64
	var readOpsPresent, writeOpsPresent, readKBPresent, writeKBPresent, queuedPresent bool
	diskRows := make([]map[string]any, 0, len(devices))
	for _, disk := range devices {
		prefix := "systemMetrics.disks." + disk + "."
		entry := map[string]any{"disk": disk}
		if reset {
			diskRows = append(diskRows, entry)
			continue
		}
		reads, readsOK := c.delta(prefix + "reads")
		writes, writesOK := c.delta(prefix + "writes")
		readSectors, readSectorsOK := c.delta(prefix + "read_sectors")
		writeSectors, writeSectorsOK := c.delta(prefix + "write_sectors")
		if !readSectorsOK {
			readSectors, readSectorsOK = c.delta(prefix + "readsSectors")
		}
		if !writeSectorsOK {
			writeSectors, writeSectorsOK = c.delta(prefix + "writesSectors")
		}
		sectorSize := 512.0
		if v, ok := c.current(prefix + "sector_size"); ok && v > 0 {
			sectorSize = v
		}
		if readsOK {
			entry["r/s"] = reads / c.dt
			totalReadOps += reads
			readOpsPresent = true
		}
		if writesOK {
			entry["w/s"] = writes / c.dt
			totalWriteOps += writes
			writeOpsPresent = true
		}
		if readSectorsOK {
			v := readSectors * sectorSize / 1024 / c.dt
			entry["rkB/s"] = v
			totalReadKB += v
			readKBPresent = true
		}
		if writeSectorsOK {
			v := writeSectors * sectorSize / 1024 / c.dt
			entry["wkB/s"] = v
			totalWriteKB += v
			writeKBPresent = true
		}
		readTime, readTimeOK := c.deltaAny(prefix+"read_time_ms", prefix+"readsMs")
		writeTime, writeTimeOK := c.deltaAny(prefix+"write_time_ms", prefix+"writesMs")
		ioTime, ioTimeOK := c.deltaAny(prefix+"io_time_ms", prefix+"ioMs")
		queued, queuedOK := c.deltaAny(prefix+"io_queued_ms", prefix+"ioMsWeighted")
		ops := reads + writes
		if queuedOK && ops > 0 {
			entry["awaitS"] = queued / ops / 1000
			totalQueuedMS += queued
			queuedPresent = true
		}
		if readTimeOK && reads > 0 {
			entry["r_awaitS"] = readTime / reads / 1000
			totalReadTimeMS += readTime
		}
		if writeTimeOK && writes > 0 {
			entry["w_awaitS"] = writeTime / writes / 1000
			totalWriteTimeMS += writeTime
		}
		if queuedOK {
			entry["aqu-sz"] = queued / (c.dt * 1000)
		}
		if ioTimeOK {
			util := ioTime / (c.dt * 1000) * 100
			if util > 100 {
				util = 100
			}
			entry["util%"] = util
			if util > maxUtil {
				maxUtil = util
			}
		}
		if v, ok := c.currentAny(prefix+"io_in_progress", prefix+"ioInProgress"); ok {
			entry["inflight"] = v
		}
		diskRows = append(diskRows, entry)
	}
	if len(diskRows) > 0 {
		row.Values["disks"] = diskRows
	}
	if readKBPresent {
		row.Values["rdMB/s"] = totalReadKB / 1024
	}
	if writeKBPresent {
		row.Values["wrMB/s"] = totalWriteKB / 1024
	}
	if readOpsPresent {
		row.Values["r/s"] = totalReadOps / c.dt
	}
	if writeOpsPresent {
		row.Values["w/s"] = totalWriteOps / c.dt
	}
	if readKBPresent {
		row.Values["rkB/s"] = totalReadKB
	}
	if writeKBPresent {
		row.Values["wkB/s"] = totalWriteKB
	}
	if maxUtil > 0 {
		row.Values["dUtil%"] = maxUtil
		row.Values["util%"] = maxUtil
	}
	if totalReadOps > 0 {
		row.Values["r_awaitS"] = totalReadTimeMS / totalReadOps / 1000
	}
	if totalWriteOps > 0 {
		row.Values["w_awaitS"] = totalWriteTimeMS / totalWriteOps / 1000
	}
	if queuedPresent && totalReadOps+totalWriteOps > 0 {
		row.Values["awaitS"] = totalQueuedMS / (totalReadOps + totalWriteOps) / 1000
	}
}

func (c calculator) deltaAny(paths ...string) (float64, bool) {
	for _, path := range paths {
		if v, ok := c.delta(path); ok {
			return v, true
		}
	}
	return 0, false
}

func fillReplication(row *Row, c calculator, members *replMemberRegistry, reset bool) {
	for label, lag := range replMemberLags(c.cur, members) {
		row.Values[label] = lag
	}
	if last, ok := c.current("serverStatus.repl.lastWrite.lastWriteDate"); ok {
		if majority, ok := c.current("serverStatus.repl.lastWrite.majorityWriteDate"); ok {
			lag := (last - majority) / 1000
			if lag >= 0 {
				row.Values["majLagS"] = lag
			}
		}
	}
	if avg, ok := averageMemberPingMs(c.cur); ok {
		row.Values["hbMs"] = avg
	}
	setCurrent(row, "applyBufCnt", c, "serverStatus.metrics.repl.buffer.apply.count")
	setCurrentMiB(row, "applyBufMB", c, "serverStatus.metrics.repl.buffer.apply.sizeBytes")
	if !reset {
		setRate(row, "applyOps/s", c, "serverStatus.metrics.repl.apply.ops")
	}
	row.Values["rsState"] = rsState(c.cur)
}

func averageMemberPingMs(sample model.MetricSample) (float64, bool) {
	var sum float64
	var count int
	for path, value := range sample.Values {
		if !strings.HasPrefix(path, "replSetGetStatus.members.") || !strings.HasSuffix(path, ".pingMs") {
			continue
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		sum += value
		count++
	}
	if count == 0 {
		return 0, false
	}
	return sum / float64(count), true
}

func processRestart(prev, cur model.MetricSample) bool {
	if p, ok := prev.GetAny("serverStatus.uptimeMillis", "serverStatus.uptime"); ok {
		if c, ok := cur.GetAny("serverStatus.uptimeMillis", "serverStatus.uptime"); ok && c < p {
			return true
		}
	}
	if p, ok := prev.Get("serverStatus.pid"); ok {
		if c, ok := cur.Get("serverStatus.pid"); ok && c != p {
			return true
		}
	}
	return false
}

func processMarker(event string, sample model.MetricSample, loc *time.Location) string {
	pid := "-"
	if v, ok := sample.Get("serverStatus.pid"); ok {
		pid = strconv.FormatInt(int64(v), 10)
	}
	start := processStart(sample, loc)
	if pid == "-" && start == "-" {
		return ""
	}
	return fmt.Sprintf("--- mongod %s: pid=%s start=%s ---", event, pid, start)
}

func processStart(sample model.MetricSample, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	if v, ok := sample.Get("serverStatus.uptimeMillis"); ok && v >= 0 {
		return sample.Time.Add(-time.Duration(v) * time.Millisecond).In(loc).Format(time.RFC3339)
	}
	if v, ok := sample.Get("serverStatus.uptime"); ok && v >= 0 {
		return sample.Time.Add(-time.Duration(v) * time.Second).In(loc).Format(time.RFC3339)
	}
	return "-"
}

func rsState(sample model.MetricSample) string {
	if v, ok := sample.GetAny("serverStatus.repl.ismaster", "serverStatus.repl.isWritablePrimary"); ok && v > 0 {
		return "PRIMARY"
	}
	if v, ok := sample.Get("serverStatus.repl.secondary"); ok && v > 0 {
		return "SECONDARY"
	}
	if v, ok := sample.Get("serverStatus.repl.myState"); ok {
		return normalizeRSState(int(v))
	}
	for path, value := range sample.Values {
		if !strings.HasPrefix(path, "replSetGetStatus.members.") || !strings.HasSuffix(path, ".self") || value <= 0 {
			continue
		}
		prefix := strings.TrimSuffix(path, ".self")
		if state, ok := sample.Get(prefix + ".state"); ok {
			return normalizeRSState(int(state))
		}
	}
	return "UNKNOWN"
}

func normalizeRSState(state int) string {
	switch state {
	case 1:
		return "PRIMARY"
	case 2:
		return "SECONDARY"
	case 3:
		return "RECOVERING"
	case 5:
		return "STARTUP2"
	case 7:
		return "ARBITER"
	default:
		return "UNKNOWN"
	}
}

func setCurrent(row *Row, key string, c calculator, path string) {
	if v, ok := c.current(path); ok {
		row.Values[key] = v
	}
}

func setRate(row *Row, key string, c calculator, path string) {
	if v, ok := c.rate(path); ok {
		row.Values[key] = v
	}
}

func setRateAny(row *Row, key string, c calculator, paths ...string) {
	if v, ok := c.rateAny(paths...); ok {
		row.Values[key] = v
	}
}

func setRateSum(row *Row, key string, c calculator, paths ...string) {
	var total float64
	var ok bool
	for _, path := range paths {
		if v, has := c.rate(path); has {
			total += v
			ok = true
		}
	}
	if ok {
		row.Values[key] = total
	}
}

func setRateMiB(row *Row, key string, c calculator, path string) {
	if v, ok := c.rate(path); ok {
		row.Values[key] = v / 1024 / 1024
	}
}

func setRateScaledAny(row *Row, key string, c calculator, scale float64, paths ...string) {
	if scale == 0 {
		return
	}
	if v, ok := c.rateAny(paths...); ok {
		row.Values[key] = v / scale
	}
}

func setPressurePercent(row *Row, key string, c calculator, resource, scope string) {
	if v, ok := c.currentAny(
		fmt.Sprintf("systemMetrics.pressure.%s.%s.avg10", resource, scope),
		fmt.Sprintf("systemMetrics.pressure.%s.%s.avg60", resource, scope),
		fmt.Sprintf("systemMetrics.pressure.%s.%s.avg300", resource, scope),
	); ok {
		row.Values[key] = v
		return
	}
	if delta, ok := c.delta(fmt.Sprintf("systemMetrics.pressure.%s.%s.totalMicros", resource, scope)); ok && c.dt > 0 {
		row.Values[key] = delta / (c.dt * 1_000_000) * 100
	}
}

func setLatency(row *Row, key string, c calculator, prefix string) {
	latency, ok := c.delta(prefix + ".latency")
	if !ok {
		return
	}
	ops, ok := c.delta(prefix + ".ops")
	if !ok || ops <= 0 {
		return
	}
	row.Values[key] = latency / ops / 1_000_000
}

func setAverage(row *Row, key string, c calculator, numeratorPath, denominatorPath string, scale float64) {
	if scale == 0 {
		scale = 1
	}
	numerator, ok := c.delta(numeratorPath)
	if !ok {
		return
	}
	denominator, ok := c.delta(denominatorPath)
	if !ok || denominator <= 0 {
		return
	}
	row.Values[key] = numerator / denominator / scale
}

func setCurrentMiB(row *Row, key string, c calculator, path string) {
	if v, ok := c.current(path); ok {
		row.Values[key] = v / 1024 / 1024
	}
}

func setBoolCurrent(row *Row, key string, c calculator, path string) {
	if v, ok := c.current(path); ok {
		if math.Abs(v) > 0 {
			row.Values[key] = float64(1)
			return
		}
		row.Values[key] = float64(0)
	}
}

func rateSumSuffix(c calculator, prefix string, suffixes []string) (float64, bool) {
	var total float64
	var ok bool
	for _, suffix := range suffixes {
		if v, has := c.rate(prefix + suffix); has {
			total += v
			ok = true
		}
	}
	return total, ok
}

func cpuCountFromMetadata(metadata model.Metadata) float64 {
	host, ok := metadata.LatestDoc("hostInfo")
	if !ok {
		return 0
	}
	for _, path := range []string{
		"system.numCoresAvailableToProcess",
		"system.numCores",
		"numCoresAvailableToProcess",
		"numCores",
	} {
		if value, ok := model.Lookup(host, path); ok {
			switch v := value.(type) {
			case int:
				if v > 0 {
					return float64(v)
				}
			case int32:
				if v > 0 {
					return float64(v)
				}
			case int64:
				if v > 0 {
					return float64(v)
				}
			case float64:
				if v > 0 {
					return v
				}
			}
		}
	}
	return 0
}

func diskDevices(sample model.MetricSample, selected string) []string {
	seen := map[string]bool{}
	for path := range sample.Values {
		if !strings.HasPrefix(path, "systemMetrics.disks.") {
			continue
		}
		rest := strings.TrimPrefix(path, "systemMetrics.disks.")
		parts := strings.SplitN(rest, ".", 2)
		if len(parts) != 2 {
			continue
		}
		if selected != "" && parts[0] != selected {
			continue
		}
		seen[parts[0]] = true
	}
	out := make([]string, 0, len(seen))
	for disk := range seen {
		out = append(out, disk)
	}
	return out
}

type replMemberRegistry struct {
	set          string
	members      []ReplMember
	labelByName  map[string]string
	labelByIndex map[int]string
	next         int
}

func newReplMemberRegistry(metadata model.Metadata) *replMemberRegistry {
	registry := &replMemberRegistry{
		labelByName:  map[string]string{},
		labelByIndex: map[int]string{},
		next:         1,
	}
	configRecords := metadataRecordsOrLatest(metadata, "replSetGetConfig")
	for _, record := range configRecords {
		config := replConfigBody(record.Doc)
		registry.set = firstNonEmpty(registry.set, lookupDocString(config, "_id"))
		for _, name := range replConfigMemberNames(config) {
			registry.addName(name)
		}
	}
	statusRecords := metadataRecordsOrLatest(metadata, "replSetGetStatus")
	for _, record := range statusRecords {
		registry.set = firstNonEmpty(registry.set, lookupDocString(record.Doc, "set"))
		for _, name := range replStatusMemberNames(record.Doc) {
			if len(configRecords) > 0 && registry.labelByName[name] != "" {
				continue
			}
			registry.addName(name)
		}
	}
	if registry.set == "" {
		if status, ok := metadata.LatestDoc("serverStatus"); ok {
			registry.set = lookupDocString(status, "repl.setName")
		}
	}
	for i, member := range registry.members {
		registry.labelByIndex[i] = member.Label
	}
	return registry
}

func ReplSetInfoFromMetadata(metadata model.Metadata) ReplSetInfo {
	registry := newReplMemberRegistry(metadata)
	return ReplSetInfo{Set: registry.set, Members: append([]ReplMember(nil), registry.members...)}
}

func metadataRecordsOrLatest(metadata model.Metadata, name string) []model.MetadataRecord {
	records := metadata.Records(name)
	if len(records) > 0 {
		return records
	}
	if record, ok := metadata.LatestRecord(name); ok {
		return []model.MetadataRecord{record}
	}
	return nil
}

func (r *replMemberRegistry) addName(name string) string {
	if name == "" || name == "-" {
		return ""
	}
	if label, ok := r.labelByName[name]; ok {
		return label
	}
	label := fmt.Sprintf("node%d", r.next)
	r.next++
	r.labelByName[name] = label
	r.members = append(r.members, ReplMember{Label: label, Name: name})
	return label
}

func (r *replMemberRegistry) labelForIndex(index int) string {
	if label, ok := r.labelByIndex[index]; ok {
		return label
	}
	label := fmt.Sprintf("node%d", r.next)
	r.next++
	r.labelByIndex[index] = label
	r.members = append(r.members, ReplMember{Label: label})
	return label
}

func replConfigBody(doc map[string]any) map[string]any {
	if value, ok := model.Lookup(doc, "config"); ok {
		if config, ok := value.(map[string]any); ok {
			return config
		}
	}
	return doc
}

func replConfigMemberNames(config map[string]any) []string {
	value, ok := model.Lookup(config, "members")
	if !ok {
		return nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		member, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := firstNonEmpty(lookupDocString(member, "host"), lookupDocString(member, "name"))
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func replStatusMemberNames(status map[string]any) []string {
	value, ok := model.Lookup(status, "members")
	if !ok {
		return nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		member, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if name := lookupDocString(member, "name"); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func lookupDocString(doc map[string]any, path string) string {
	value, ok := model.Lookup(doc, path)
	if !ok {
		return ""
	}
	text, ok := model.AsString(value)
	if !ok || text == "" || text == "-" {
		return ""
	}
	return text
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" && value != "-" {
			return value
		}
	}
	return ""
}

func replMemberLags(sample model.MetricSample, members *replMemberRegistry) map[string]float64 {
	indexes := replMemberIndexes(sample)
	for _, index := range indexes {
		members.labelForIndex(index)
	}
	primaryIndex, ok := primaryMemberIndex(sample, indexes)
	if !ok {
		return nil
	}
	lags := map[string]float64{}
	primaryLabel := members.labelForIndex(primaryIndex)
	lags[primaryLabel] = 0
	primaryDate, primaryDateOK := memberOptimeDate(sample, primaryIndex)
	primaryTS, primaryTSOK := memberOptimeTimestamp(sample, primaryIndex)
	for _, index := range indexes {
		label := members.labelForIndex(index)
		if index == primaryIndex {
			lags[label] = 0
			continue
		}
		if primaryDateOK {
			if optime, ok := memberOptimeDate(sample, index); ok {
				lags[label] = nonNegativeLag((primaryDate - optime) / 1000)
				continue
			}
		}
		if primaryTSOK {
			if optime, ok := memberOptimeTimestamp(sample, index); ok {
				lags[label] = nonNegativeLag(primaryTS - optime)
			}
		}
	}
	return lags
}

func replMemberIndexes(sample model.MetricSample) []int {
	seen := map[int]bool{}
	for path, value := range sample.Values {
		if value == 0 || !strings.HasPrefix(path, "replSetGetStatus.members.") {
			continue
		}
		rest := strings.TrimPrefix(path, "replSetGetStatus.members.")
		parts := strings.SplitN(rest, ".", 2)
		if len(parts) != 2 {
			continue
		}
		index, err := strconv.Atoi(parts[0])
		if err != nil || index < 0 {
			continue
		}
		seen[index] = true
	}
	out := make([]int, 0, len(seen))
	for index := range seen {
		out = append(out, index)
	}
	sort.Ints(out)
	return out
}

func primaryMemberIndex(sample model.MetricSample, indexes []int) (int, bool) {
	for _, index := range indexes {
		if state, ok := sample.Get(fmt.Sprintf("replSetGetStatus.members.%d.state", index)); ok && int(state) == 1 {
			return index, true
		}
	}
	return 0, false
}

func memberOptimeDate(sample model.MetricSample, index int) (float64, bool) {
	value, ok := sample.Get(fmt.Sprintf("replSetGetStatus.members.%d.optimeDate", index))
	return value, ok && value > 0
}

func memberOptimeTimestamp(sample model.MetricSample, index int) (float64, bool) {
	value, ok := sample.Get(fmt.Sprintf("replSetGetStatus.members.%d.optime.ts.t", index))
	return value, ok && value > 0
}

func nonNegativeLag(lag float64) float64 {
	if lag < 0 {
		return 0
	}
	return lag
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
