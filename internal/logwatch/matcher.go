package logwatch

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/fabienpiette/schedule-containers/internal/models"
)

// Matcher tests a single log line against a rule's pattern.
type Matcher interface {
	Matches(line string) bool
}

type substringMatcher struct{ s string }

func (m substringMatcher) Matches(line string) bool { return strings.Contains(line, m.s) }

type regexMatcher struct{ re *regexp.Regexp }

func (m regexMatcher) Matches(line string) bool { return m.re.MatchString(line) }

// NewMatcher compiles a matcher once, at rule load/update time.
func NewMatcher(pattern, matchType string) (Matcher, error) {
	switch matchType {
	case models.MatchSubstring:
		return substringMatcher{s: pattern}, nil
	case models.MatchRegex:
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
		return regexMatcher{re: re}, nil
	default:
		return nil, fmt.Errorf("unknown match type %q", matchType)
	}
}
