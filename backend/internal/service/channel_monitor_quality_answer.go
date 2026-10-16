package service

import (
	"math/big"
	"regexp"
	"strings"
)

var (
	monitorQualityNumber       = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)
	monitorQualityAnswerPrefix = regexp.MustCompile(`(?i)^(?:(?:因此|所以|综上)[，,：:]?\s*)?(?:(?:最终答案|正确答案|答案|结果)\s*(?:是|为|[:：])\s*[:：]?\s*|(?:最少|至少)\s*(?:需要)?\s*(?:取出|取|摸出|抽取)?\s*[:：]?\s*|(?:the\s+)?(?:final\s+)?answer\s*(?:is\s*:?|:)\s*)`)
	// Only a complete numeric conclusion, optionally with a count unit, is
	// accepted. A decimal point followed by digits is not a sentence boundary.
	monitorQualityNumericConclusion = regexp.MustCompile(`(?i)^([+-]?[0-9]+(?:\.[0-9]+)?)(?:\s*(?:个|颗|枚)(?:糖果)?|\s+(?:candies|pieces))?\s*(?:$|[。！!：:]|\.(?:\s|$))`)
	monitorQualityNumberFormatting  = strings.NewReplacer("**", "", "__", "", "`", "")
)

// matchMonitorQualityAnswer compares answers, not arbitrary substrings of the
// explanation. Numeric checks additionally accept a clearly stated conclusion
// such as "最少取出 **21个**：...". This is deliberately not a semantic grader:
// unsupported prose still requires a concise answer in the requested format.
func matchMonitorQualityAnswer(response, expected string) bool {
	response = strings.TrimSpace(response)
	expected = strings.TrimSpace(expected)
	if response == expected {
		return true
	}
	if unwrapMonitorQualityAnswer(response) == expected {
		return true
	}
	if !monitorQualityNumber.MatchString(expected) {
		return false
	}
	want, ok := new(big.Rat).SetString(expected)
	if !ok {
		return false
	}

	found, first := false, true
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "# \t")
		line = monitorQualityNumberFormatting.Replace(line)
		prefix := monitorQualityAnswerPrefix.FindString(line)
		isFirst := first
		first = false
		if prefix == "" && !isFirst {
			continue
		}
		match := monitorQualityNumericConclusion.FindStringSubmatch(strings.TrimPrefix(line, prefix))
		if len(match) == 0 {
			if prefix != "" {
				// An explicit but ambiguous conclusion must not be overridden by
				// another line that happens to contain the expected answer.
				return false
			}
			continue
		}
		got, ok := new(big.Rat).SetString(match[1])
		if !ok || got.Cmp(want) != 0 {
			return false
		}
		found = true
	}
	return found
}

func unwrapMonitorQualityAnswer(answer string) string {
	for _, marker := range []string{"**", "__", "`"} {
		if len(answer) > 2*len(marker) && strings.HasPrefix(answer, marker) && strings.HasSuffix(answer, marker) {
			return strings.TrimSpace(answer[len(marker) : len(answer)-len(marker)])
		}
	}
	return answer
}
