package macos

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Version represents a parsed macOS version number.
type Version struct {
	Major, Minor, Patch int
}

// Detect runs sw_vers and returns the parsed macOS version.
func Detect() (Version, error) {
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return Version{}, fmt.Errorf("sw_vers failed: %w", err)
	}
	return parse(strings.TrimSpace(string(out)))
}

func parse(s string) (Version, error) {
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return Version{}, fmt.Errorf("unexpected version format: %q", s)
	}
	var v Version
	var err error
	v.Major, err = strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version in %q: %w", s, err)
	}
	v.Minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version in %q: %w", s, err)
	}
	if len(parts) >= 3 {
		v.Patch, _ = strconv.Atoi(parts[2])
	}
	return v, nil
}

// AtLeast returns true if the version is >= major.minor.
func (v Version) AtLeast(major, minor int) bool {
	return v.Major > major || (v.Major == major && v.Minor >= minor)
}

// String returns the dotted version string.
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// MarketingName returns the Apple marketing name for the major version.
func (v Version) MarketingName() string {
	names := map[int]string{
		10: "Legacy", // 10.x handled separately
		11: "Big Sur",
		12: "Monterey",
		13: "Ventura",
		14: "Sonoma",
		15: "Sequoia",
	}
	if v.Major == 10 {
		legacyNames := map[int]string{
			13: "High Sierra",
			14: "Mojave",
			15: "Catalina",
		}
		if name, ok := legacyNames[v.Minor]; ok {
			return name
		}
		return "macOS 10.x"
	}
	if name, ok := names[v.Major]; ok {
		return name
	}
	return ""
}
