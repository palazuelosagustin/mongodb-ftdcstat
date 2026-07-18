package model

import (
	"fmt"
	"time"
)

type ReplMemberState struct {
	Label string
	Name  string
}

func (m Metadata) ProcessKind() string {
	return m.processKind
}

func (m Metadata) StorageEngineName() string {
	return m.storageEngineName
}

func (m Metadata) ReplSetSnapshot() (string, []ReplMemberState) {
	if len(m.replMembers) == 0 && m.replSetName == "" {
		return "", nil
	}
	members := append([]ReplMemberState(nil), m.replMembers...)
	return m.replSetName, members
}

func (m Metadata) compactServerStatusDoc() (map[string]any, bool) {
	doc := map[string]any{}
	if m.storageEngineName != "" {
		doc["storageEngine"] = map[string]any{"name": m.storageEngineName}
	}
	if m.replSetName != "" {
		doc["repl"] = map[string]any{"setName": m.replSetName}
	}
	if len(doc) == 0 {
		return nil, false
	}
	return doc, true
}

func (m *Metadata) captureServerStatus(record MetadataRecord) {
	m.maybeSetNetworkMaxConn(record)
	if process := lookupMetadataString(record.Doc, "process"); process != "" {
		m.setProcessKind(process)
	}
	if name := lookupMetadataString(record.Doc, "storageEngine.name"); name != "" {
		if m.shouldPreferNewerScalar(record.Timestamp, m.storageEngineTime) {
			m.storageEngineName = name
			m.storageEngineTime = record.Timestamp
		}
	}
	if set := lookupMetadataString(record.Doc, "repl.setName"); set != "" && m.replSetName == "" {
		m.replSetName = set
	}
}

func (m *Metadata) captureReplSetGetStatus(record MetadataRecord) {
	if set := lookupMetadataString(record.Doc, "set"); set != "" && m.replSetName == "" {
		m.replSetName = set
	}
	for _, name := range replStatusMemberNames(record.Doc) {
		m.addReplMemberName(name, false)
	}
}

func (m *Metadata) captureReplSetGetConfig(record MetadataRecord) {
	config := replConfigBody(record.Doc)
	if set := lookupMetadataString(config, "_id"); set != "" {
		m.replSetName = set
	}
	for _, name := range replConfigMemberNames(config) {
		m.addReplMemberName(name, true)
	}
	m.updateLatest(record)
}

func (m *Metadata) addReplMemberName(name string, fromConfig bool) {
	if name == "" {
		return
	}
	if m.replMemberByName == nil {
		m.replMemberByName = map[string]string{}
	}
	if label, ok := m.replMemberByName[name]; ok {
		if fromConfig {
			return
		}
		_ = label
		return
	}
	if !fromConfig && len(m.replMembers) > 0 {
		// Status-only names should not override config-derived members.
		for _, member := range m.replMembers {
			if member.Name == name {
				return
			}
		}
	}
	label := fmt.Sprintf("node%d", m.replNextLabel)
	m.replNextLabel++
	m.replMemberByName[name] = label
	m.replMembers = append(m.replMembers, ReplMemberState{Label: label, Name: name})
}

func (m *Metadata) updateLatest(record MetadataRecord) {
	old, exists := m.Latest[record.Name]
	if exists && !record.Timestamp.IsZero() && !old.Timestamp.IsZero() && record.Timestamp.Before(old.Timestamp) {
		return
	}
	m.Latest[record.Name] = record
}

func (m *Metadata) shouldPreferNewerScalar(recordTs, currentTs time.Time) bool {
	if recordTs.IsZero() {
		return currentTs.IsZero()
	}
	if currentTs.IsZero() {
		return true
	}
	return !recordTs.Before(currentTs)
}

func replConfigBody(doc map[string]any) map[string]any {
	if value, ok := Lookup(doc, "config"); ok {
		if config, ok := value.(map[string]any); ok {
			return config
		}
	}
	return doc
}

func replConfigMemberNames(config map[string]any) []string {
	value, ok := Lookup(config, "members")
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
		name := firstMetadataString(
			lookupMetadataString(member, "host"),
			lookupMetadataString(member, "name"),
		)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func replStatusMemberNames(status map[string]any) []string {
	value, ok := Lookup(status, "members")
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
		if name := lookupMetadataString(member, "name"); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func lookupMetadataString(doc map[string]any, path string) string {
	value, ok := Lookup(doc, path)
	if !ok {
		return ""
	}
	text, ok := AsString(value)
	if !ok || text == "" || text == "-" {
		return ""
	}
	return text
}

func firstMetadataString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (m *Metadata) normalizeRootDocument(root map[string]any) map[string]any {
	if common, ok := root["common"].(map[string]any); ok {
		normalized := map[string]any{}
		for key, value := range root {
			if key == "common" {
				continue
			}
			normalized[key] = value
		}
		for key, value := range common {
			normalized[key] = value
		}
		root = normalized
	}
	return root
}

func (m *Metadata) setProcessKind(kind string) {
	switch kind {
	case ProcessKindMongod:
		m.processKind = ProcessKindMongod
	case ProcessKindMongos:
		m.processKind = ProcessKindMongos
	}
}
