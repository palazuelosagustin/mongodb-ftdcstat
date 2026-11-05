package render

import (
	"sort"
	"strconv"
	"strings"

	"mongodb-ftdcstat/internal/model"
)

// MetadataSummary is the compact "metadata" object of the JSON outputs. It
// holds the same facts as the CLI header tables (Report process, Host, Build,
// Command Line Options, Parameters), typed, instead of the raw FTDC metadata
// documents, which are tens of KB of mostly defaults. Empty fields are left
// out. The replica set is reported separately as "rsInfo".
func MetadataSummary(metadata model.Metadata) map[string]any {
	build, _ := metadata.LatestDoc("buildInfo")
	host, _ := metadata.LatestDoc("hostInfo")
	cmd, _ := metadata.LatestDoc("getCmdLineOpts")
	params, _ := metadata.LatestDoc("getParameter")
	status, _ := metadata.LatestDoc("serverStatus")

	out := map[string]any{"process": reportProcessKind(metadata)}

	hostOut := map[string]any{}
	putString(hostOut, "hostname", lookupFirst(host, "system.hostname", "hostname"))
	putString(hostOut, "os", hostOS(host))
	putString(hostOut, "kernel", lookupFirst(host, "kernelVersion", "system.kernelVersion", "extra.kernelVersion"))
	putString(hostOut, "arch", lookupFirst(host, "system.cpuArch", "cpuArch", "extra.cpuArch"))
	putNumber(hostOut, "cpuCores", lookupFirst(host, "system.numCores", "numCores", "extra.numCores"))
	putNumber(hostOut, "memoryMB", lookupFirst(host, "system.memSizeMB", "memSizeMB", "extra.memSizeMB"))
	putNumber(hostOut, "maxConnections", metadata.NetworkMaxConnDisplay())
	putMap(out, "host", hostOut)

	buildOut := map[string]any{}
	putString(buildOut, "version", lookupString(build, "version"))
	putString(buildOut, "gitVersion", lookupString(build, "gitVersion"))
	putList(buildOut, "modules", lookupList(build, "modules"))
	putString(buildOut, "storageEngine", firstString(metadata.StorageEngineName(), lookupString(status, "storageEngine.name"), lookupString(cmd, "parsed.storage.engine")))
	putString(buildOut, "allocator", lookupString(build, "allocator"))
	putString(buildOut, "openssl", lookupString(build, "openssl.running"))
	putList(buildOut, "perconaFeatures", lookupUniqueList(build, "perconaFeatures"))
	putMap(out, "build", buildOut)

	if parsed, ok := model.Lookup(cmd, "parsed"); ok {
		if parsedMap, ok := parsed.(map[string]any); ok {
			opts := map[string]any{}
			for key, value := range parsedMap {
				if key == "setParameter" {
					continue
				}
				flattenTyped(opts, value, key, 4)
			}
			putMap(out, "cmdLineOpts", opts)
		}
	}

	paramsOut := map[string]any{}
	if v, ok := model.Lookup(params, "wiredTigerEngineRuntimeConfig.cache_size"); ok {
		paramsOut["wtCache"] = v
	} else if v, ok := model.Lookup(cmd, "parsed.storage.wiredTiger.engineConfig.cacheSizeGB"); ok {
		paramsOut["wtCache"] = v
	}
	if setParameter, ok := model.Lookup(cmd, "parsed.setParameter"); ok {
		if m, ok := setParameter.(map[string]any); ok {
			for key, value := range m {
				flattenTyped(paramsOut, value, key, 2)
			}
		}
	}
	putMap(out, "parameters", paramsOut)
	return out
}

// flattenTyped adds the leaves of value to out under dotted keys, keeping
// their JSON types. Lists are kept as lists.
func flattenTyped(out map[string]any, value any, key string, depth int) {
	if m, ok := value.(map[string]any); ok && depth > 0 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			flattenTyped(out, m[k], key+"."+k, depth-1)
		}
		return
	}
	out[key] = value
}

func putString(out map[string]any, key, value string) {
	if value != "" && value != "-" {
		out[key] = value
	}
}

func putNumber(out map[string]any, key, value string) {
	if n, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
		out[key] = n
	}
}

func putList(out map[string]any, key, commaList string) {
	if commaList == "" || commaList == "-" {
		return
	}
	out[key] = strings.Split(commaList, ",")
}

func putMap(out map[string]any, key string, value map[string]any) {
	if len(value) > 0 {
		out[key] = value
	}
}
