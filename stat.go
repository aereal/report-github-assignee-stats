package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/tidwall/gjson"
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

func StatsFor(kind Assignable, jsonResult gjson.Result) AssignedIssuesStat {
	var kindName string
	if kind == Issue {
		kindName = "issues"
	} else if kind == PullRequest {
		kindName = "pullRequests"
	} else {
		// no-op
	}

	assignedIssuesStat := make(AssignedIssuesStat)
	got := jsonResult.Get(fmt.Sprintf("data.repository.%s.nodes.#.assignees.nodes", kindName))
	for _, as := range got.Array() {
		assignees := as.Array()
		if len(assignees) == 0 {
			assignedIssuesStat["_nobody"]++
		} else {
			for _, assignee := range assignees {
				assigneeName := assignee.Get("login").String()
				assignedIssuesStat[assigneeName]++
			}
		}
	}

	return assignedIssuesStat
}

func MergeStats(stats []AssignedIssuesStat) AssignedIssuesStat {
	total := make(AssignedIssuesStat)
	for _, st := range stats {
		for name, count := range st {
			total[name] += count
		}
	}
	return total
}
