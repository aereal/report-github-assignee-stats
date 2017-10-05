package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	mkr "github.com/mackerelio/mackerel-client-go"
	"github.com/tidwall/gjson"
)

type Assignable int

const (
	Issue Assignable = iota
	PullRequest
)

type environment struct {
	GitHubEndpoint string
	GitHubToken    string
	MackerelApiKey string
	RepoName       string
	Owner          string
}

func NewEnvironment(owner string, repoName string, githubEndpoint string, githubToken string, mackerelApiKey string) (*environment, error) {
	if owner == "" {
		return nil, fmt.Errorf("owner required")
	}
	if repoName == "" {
		return nil, fmt.Errorf("repoName required")
	}
	if mackerelApiKey == "" {
		return nil, fmt.Errorf("mackerelApiKey required")
	}
	if githubToken == "" {
		return nil, fmt.Errorf("githubToken required")
	}
	if githubEndpoint == "" {
		return nil, fmt.Errorf("githubEndpoint required")
	}
	env := &environment{
		Owner:          owner,
		RepoName:       repoName,
		GitHubEndpoint: githubEndpoint,
		GitHubToken:    githubToken,
		MackerelApiKey: mackerelApiKey,
	}
	return env, nil
}

type GitHubGraphqlRequest struct {
	Query string `json:"query"`
}

type AssignedIssuesStat map[string]int

func main() {
	var (
		owner          string
		repoName       string
		githubEndpoint string
		githubToken    string
		mackerelApiKey string
	)
	flag.StringVar(&owner, "owner", "", "repository owner name")
	flag.StringVar(&repoName, "repo", "", "repository name")
	flag.StringVar(&githubEndpoint, "github-endpoint", "https://api.github.com", "GitHub GraphQL endpoint")
	flag.StringVar(&githubToken, "github-token", "", "GitHub API token")
	flag.StringVar(&mackerelApiKey, "mackerel-api-key", "", "Mackerel API key")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -owner=<repositoryOwner> -repo=<repositoryName> -github-token=<githubApiToken> -mackerel-api-key=<mackerelApiKey> [-github-endpoint=<githubEndpoint>]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	env, err := NewEnvironment(owner, repoName, githubEndpoint, githubToken, mackerelApiKey)
	if err != nil {
		onError(err)
	}
	env.run()
}

type Paging struct {
	EndCursor   string
	HasNextPage bool
}

func (p *Paging) asQuery() string {
	if p.EndCursor == "" {
		return ""
	}
	return fmt.Sprintf(", after: %#v", p.EndCursor)
}

func onError(err error) {
	log.Fatalf("Error: %s", err)
	os.Exit(1)
}

func (env *environment) buildQueryFor(kind Assignable, paging *Paging) string {
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

func (env *environment) buildQuery(issuesPaging *Paging, prsPaging *Paging) (*GitHubGraphqlRequest, error) {
	issuesQuery := env.buildQueryFor(Issue, issuesPaging)
	prsQuery := env.buildQueryFor(PullRequest, prsPaging)
	qs := fmt.Sprintf(`
query {
  repository(owner: "%s", name: "%s") {
		%s
		%s
  }
}
	`, env.Owner, env.RepoName, issuesQuery, prsQuery)
	query := &GitHubGraphqlRequest{
		Query: qs,
	}
	return query, nil
}

func buildRequestForGraphQL(env *environment, query *GitHubGraphqlRequest) (*http.Request, error) {
	reqBody, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/graphql", env.GitHubEndpoint), strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("bearer %s", env.GitHubToken))
	return req, nil
}

func request(req *http.Request) ([]byte, error) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func buildAssigneesStats(assignedIssues AssignedIssuesStat) string {
	buf := ""
	var keys []string
	for k := range assignedIssues {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		count := assignedIssues[name]
		buf += fmt.Sprintf("%s: %v\n", name, count)
	}
	return buf
}

func (e *environment) reportAssigneesStats(assignedIssues AssignedIssuesStat) error {
	fmt.Fprintf(os.Stdout, buildAssigneesStats(assignedIssues))

	var metricValues []*mkr.MetricValue
	now := time.Now().Unix()
	for name, count := range assignedIssues {
		value := &mkr.MetricValue{
			Name:  fmt.Sprintf("assigned_tasks_count.%s", name),
			Value: count,
			Time:  now,
		}
		metricValues = append(metricValues, value)
	}
	client := mkr.NewClient(e.MackerelApiKey)
	err := client.PostServiceMetricValues("Hatena-Blog", metricValues)
	return err
}

func statsFor(kind Assignable, jsonResult gjson.Result) AssignedIssuesStat {
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

func mergeStats(stats []AssignedIssuesStat) AssignedIssuesStat {
	total := make(AssignedIssuesStat)
	for _, st := range stats {
		for name, count := range st {
			total[name] += count
		}
	}
	return total
}

func (e *environment) run() {
	var issuesPaging *Paging
	var prsPaging *Paging
	currentPage := 1
	assignedIssues := make(AssignedIssuesStat)
	for {
		log.Printf("---> Get #%v ...\n", currentPage)
		query, err := e.buildQuery(issuesPaging, prsPaging)
		if err != nil {
			onError(err)
		}
		req, err := buildRequestForGraphQL(e, query)
		if err != nil {
			onError(err)
		}
		body, err := request(req)
		if err != nil {
			onError(err)
		}
		parsed := gjson.ParseBytes(body)
		issuesStats := statsFor(Issue, parsed)
		prsStats := statsFor(PullRequest, parsed)
		assignedIssues = mergeStats([]AssignedIssuesStat{
			assignedIssues,
			issuesStats,
			prsStats,
		})

		issuesPageInfo := parsed.Get("data.repository.issues.pageInfo")
		prsPageInfo := parsed.Get("data.repository.pullRequests.pageInfo")
		log.Printf("%#v: has next page = %#v; end cursor = %#v\n", issuesPageInfo, issuesPageInfo.Get("hasNextPage").Bool(), issuesPageInfo.Get("endCursor").String())
		if !issuesPageInfo.Get("hasNextPage").Bool() && !prsPageInfo.Get("hasNextPage").Bool() {
			break
		}
		issuesPaging = &Paging{HasNextPage: issuesPageInfo.Get("hasNextPage").Bool(), EndCursor: issuesPageInfo.Get("endCursor").String()}
		prsPaging = &Paging{HasNextPage: prsPageInfo.Get("hasNextPage").Bool(), EndCursor: prsPageInfo.Get("endCursor").String()}
		currentPage++
		time.Sleep(1 * time.Second)
	}
	err := e.reportAssigneesStats(assignedIssues)
	if err != nil {
		onError(err)
	}
}
