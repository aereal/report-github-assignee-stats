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
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

type Assignable int

const (
	Issue Assignable = iota
	PullRequest
)

func main() {
	var (
		owner          string
		repoName       string
		githubEndpoint string
		githubToken    string
		metricPrefix   string
	)
	flag.StringVar(&owner, "owner", "", "repository owner name")
	flag.StringVar(&repoName, "repo", "", "repository name")
	flag.StringVar(&githubEndpoint, "github-endpoint", "https://api.github.com", "GitHub GraphQL endpoint")
	flag.StringVar(&githubToken, "github-token", "", "GitHub API token")
	flag.StringVar(&metricPrefix, "metric-prefix", "assigned_issues_count", "Prefix name")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -owner=<repositoryOwner> -repo=<repositoryName> -github-token=<githubApiToken> [-github-endpoint=<githubEndpoint>] [-metric-prefix=<prefixString>]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	env, err := NewEnvironment(owner, repoName, githubEndpoint, githubToken, metricPrefix)
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

func buildRequestForGraphQL(env *Environment, query *GitHubGraphqlRequest) (*http.Request, error) {
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

func (e *Environment) run() {
	var issuesPaging *Paging
	var prsPaging *Paging
	currentPage := 1
	assignedIssues := make(AssignedIssuesStat)
	for {
		query, err := BuildQuery(e.Owner, e.RepoName, issuesPaging, prsPaging)
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
		issuesStats := StatsFor(Issue, parsed)
		prsStats := StatsFor(PullRequest, parsed)
		assignedIssues = MergeStats([]AssignedIssuesStat{
			assignedIssues,
			issuesStats,
			prsStats,
		})

		issuesPageInfo := parsed.Get("data.repository.issues.pageInfo")
		prsPageInfo := parsed.Get("data.repository.pullRequests.pageInfo")
		if !issuesPageInfo.Get("hasNextPage").Bool() && !prsPageInfo.Get("hasNextPage").Bool() {
			break
		}
		issuesPaging = &Paging{HasNextPage: issuesPageInfo.Get("hasNextPage").Bool(), EndCursor: issuesPageInfo.Get("endCursor").String()}
		prsPaging = &Paging{HasNextPage: prsPageInfo.Get("hasNextPage").Bool(), EndCursor: prsPageInfo.Get("endCursor").String()}
		currentPage++
		time.Sleep(1 * time.Second)
	}

	fmt.Fprintf(os.Stdout, assignedIssues.asMetric(e.MetricPrefix))
}
