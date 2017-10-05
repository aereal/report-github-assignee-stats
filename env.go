package main

import "fmt"

type Environment struct {
	GitHubEndpoint string
	GitHubToken    string
	RepoName       string
	Owner          string
	MetricPrefix   string
}

func NewEnvironment(owner string, repoName string, githubEndpoint string, githubToken string, metricPrefix string) (*Environment, error) {
	if owner == "" {
		return nil, fmt.Errorf("owner required")
	}
	if repoName == "" {
		return nil, fmt.Errorf("repoName required")
	}
	if githubToken == "" {
		return nil, fmt.Errorf("githubToken required")
	}
	if githubEndpoint == "" {
		return nil, fmt.Errorf("githubEndpoint required")
	}
	if metricPrefix == "" {
		return nil, fmt.Errorf("metricPrefix required")
	}
	env := &Environment{
		Owner:          owner,
		RepoName:       repoName,
		GitHubEndpoint: githubEndpoint,
		GitHubToken:    githubToken,
		MetricPrefix:   metricPrefix,
	}
	return env, nil
}
