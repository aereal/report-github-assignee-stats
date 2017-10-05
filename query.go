package main

import "fmt"

type GitHubGraphqlRequest struct {
	Query string `json:"query"`
}

func BuildQuery(owner string, repoName string, issuesPaging *Paging, prsPaging *Paging) (*GitHubGraphqlRequest, error) {
	issuesQuery := buildQueryFor(Issue, issuesPaging)
	prsQuery := buildQueryFor(PullRequest, prsPaging)
	qs := fmt.Sprintf(`
query {
  repository(owner: "%s", name: "%s") {
		%s
		%s
  }
}
	`, owner, repoName, issuesQuery, prsQuery)
	query := &GitHubGraphqlRequest{
		Query: qs,
	}
	return query, nil
}

func buildQueryFor(kind Assignable, paging *Paging) string {
	var pagingQuery string
	if paging != nil {
		pagingQuery = paging.asQuery()
	} else {
		pagingQuery = ""
	}

	var connection string
	if kind == Issue {
		connection = "issues"
	} else if kind == PullRequest {
		connection = "pullRequests"
	} else {
		// no-op
	}

	q := fmt.Sprintf(`
		%s(first: 100, states: [OPEN] %s) {
			pageInfo {
				hasNextPage
				endCursor
			}
			nodes {
				assignees(first: 10) {
					nodes {
						login
					}
				}
			}
		}
	`, connection, pagingQuery)
	return q
}
