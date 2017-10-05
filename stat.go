package main

import (
	"fmt"
	"sort"
	"time"
)

type AssignedIssuesStat map[string]int

func (s AssignedIssuesStat) asMetric(prefix string) string {
	now := time.Now().Unix()
	buf := ""
	var keys []string
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		count := s[name]
		buf += fmt.Sprintf("%s.%s %v %v\n", prefix, name, count, now)
	}
	return buf
}
