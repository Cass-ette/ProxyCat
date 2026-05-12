package selfupdate

import (
	"fmt"
	"regexp"
	"strconv"
)

var strictVersionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

type version struct {
	major int
	minor int
	patch int
}

func parseVersion(raw string) (version, error) {
	match := strictVersionPattern.FindStringSubmatch(raw)
	if match == nil {
		return version{}, fmt.Errorf("invalid version: %s", raw)
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return version{major: major, minor: minor, patch: patch}, nil
}

func (v version) compare(other version) int {
	if v.major != other.major {
		return compareInt(v.major, other.major)
	}
	if v.minor != other.minor {
		return compareInt(v.minor, other.minor)
	}
	return compareInt(v.patch, other.patch)
}

func compareInt(a int, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
