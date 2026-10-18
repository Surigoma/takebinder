package main

import (
	"path/filepath"
	"regexp"
	"strings"
)

// DJI filenames are only a discovery hint: other DJI cameras use them too,
// and adjacent counters do not establish that files belong to one recording.
var djiName = regexp.MustCompile(`(?i)^DJI_[A-Z0-9_]+\.MP4$`)

func djiClassification(probe probeResult, name string) string {
	model := metadataValue(probe, "model", "camera_model", "com.apple.quicktime.model")
	normalized := strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(model))
	switch normalized {
	case "osmopocket", "djiosmopocket", "osmopocket1", "djiosmopocket1",
		"pocket2", "djipocket2", "osmopocket2", "djiosmopocket2",
		"osmopocket3", "djiosmopocket3", "osmopocket4", "djiosmopocket4":
		return "osmo-pocket"
	}
	make := metadataValue(probe, "make", "manufacturer", "com.apple.quicktime.make")
	if strings.EqualFold(make, "DJI") || djiName.MatchString(name) {
		return "dji-candidate"
	}
	return ""
}

func isDJIClassification(classification string) bool {
	return classification == "osmo-pocket" || classification == "dji-candidate"
}

func djiFileIdentity(path string) (key string, chapter int, basis, confidence string) {
	// Use the full path, not a filename counter, timestamp, or generic Capture ID.
	// Every DJI file is an independent copy until capture boundaries and DJI
	// metadata preservation during remuxing have been validated with real media.
	return "dji-file|" + filepath.Clean(path), 1, "DJI file copy", "low"
}
