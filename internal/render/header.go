package render

import (
	"bytes"
	"time"

	"ftdcstat/internal/derive"
	"ftdcstat/internal/model"
)

func HeaderText(metadata model.Metadata, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	rsInfo := derive.ReplSetInfoFromMetadata(metadata)
	var buf bytes.Buffer
	renderHeader(&buf, metadata, rsInfo, loc)
	return buf.String()
}
