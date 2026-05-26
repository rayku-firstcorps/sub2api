package service

import "unicode/utf8"

type keywordFilterMatcher struct {
	nodes []keywordFilterACNode
}

type keywordFilterACNode struct {
	Next    map[rune]int
	Fail    int
	Outputs []string
}

type keywordFilterACMatch struct {
	Pattern string
	Start   int
	End     int
}

func newKeywordFilterMatcher(patterns []string) *keywordFilterMatcher {
	m := &keywordFilterMatcher{nodes: []keywordFilterACNode{{Next: map[rune]int{}}}}
	for _, pattern := range patterns {
		m.add(pattern)
	}
	m.build()
	return m
}

func (m *keywordFilterMatcher) add(pattern string) {
	if pattern == "" {
		return
	}
	state := 0
	for _, r := range pattern {
		if m.nodes[state].Next == nil {
			m.nodes[state].Next = map[rune]int{}
		}
		next, ok := m.nodes[state].Next[r]
		if !ok {
			next = len(m.nodes)
			m.nodes[state].Next[r] = next
			m.nodes = append(m.nodes, keywordFilterACNode{Next: map[rune]int{}})
		}
		state = next
	}
	m.nodes[state].Outputs = append(m.nodes[state].Outputs, pattern)
}

func (m *keywordFilterMatcher) build() {
	if m == nil || len(m.nodes) == 0 {
		return
	}
	queue := make([]int, 0)
	for _, next := range m.nodes[0].Next {
		queue = append(queue, next)
	}
	for head := 0; head < len(queue); head++ {
		state := queue[head]
		for r, next := range m.nodes[state].Next {
			fail := m.nodes[state].Fail
			for fail != 0 {
				if candidate, ok := m.nodes[fail].Next[r]; ok {
					fail = candidate
					break
				}
				fail = m.nodes[fail].Fail
			}
			if fail == 0 {
				if candidate, ok := m.nodes[0].Next[r]; ok && candidate != next {
					fail = candidate
				}
			}
			m.nodes[next].Fail = fail
			if len(m.nodes[fail].Outputs) > 0 {
				m.nodes[next].Outputs = append(m.nodes[next].Outputs, m.nodes[fail].Outputs...)
			}
			queue = append(queue, next)
		}
	}
}

func (m *keywordFilterMatcher) Scan(text string, fn func(keywordFilterACMatch) bool) {
	if m == nil || len(m.nodes) == 0 || text == "" || fn == nil {
		return
	}
	state := 0
	for idx, r := range text {
		for state != 0 {
			if _, ok := m.nodes[state].Next[r]; ok {
				break
			}
			state = m.nodes[state].Fail
		}
		if next, ok := m.nodes[state].Next[r]; ok {
			state = next
		}
		if len(m.nodes[state].Outputs) == 0 {
			continue
		}
		end := idx + utf8.RuneLen(r)
		for _, pattern := range m.nodes[state].Outputs {
			if !fn(keywordFilterACMatch{
				Pattern: pattern,
				Start:   end - len(pattern),
				End:     end,
			}) {
				return
			}
		}
	}
}
