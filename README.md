[![main](https://github.com/isometry/gh-promotion-app/actions/workflows/main.yaml/badge.svg?branch=main)](https://github.com/isometry/gh-promotion-app/actions/workflows/main.yaml)
<br>

<p align="center" width="100%">
    <img src="https://github.com/isometry/gh-promotion-app/blob/main/docs/images/banner.png?raw=true" width="220"></img>
    <br>
    <i>gitops.kyverno-policies</i>
    <br>
    🔎 <a href="#how-it-works">How it works</a> | 👍 <a href="#contributing">Contributing</a>
    <br><br>
    🪄 <a href="https://github.com/isometry/gh-promotion-app">gh-promotion-app</a>
</p>

## How it works

The `gh-promotion-app` is a service that automates the promotion of GitHub branch across environments. It is designed to
operate as a GitHub App and respond to the webhook events to which its App is subscribed.
It currently supports the following event types:

| Event Type            | Description                                     |
|-----------------------|-------------------------------------------------|
| `push`                | Change is pushed to a given branch              |
| `pull_request`        | Pull request is opened, closed, or synchronized |
| `pull_request_review` | Pull request review is submitted                |
| `check_suite`         | Check suite is completed                        |
| `deployment_status`   | Deployment status is completed                  |
| `status`              | When the status of a Git commit changes         |
| `workflow_run`        | Workflow run is completed                       |

### Overview

<img src="https://github.com/isometry/gh-promotion-app/blob/main/docs/images/overview.png?raw=true" width="500"></img>

### Usage

```console
Usage:
   [flags]
   [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  lambda      
  service     

Flags:
  -A, --github.auth-serviceMode string   [GITHUB_AUTH_MODE] Authentication serviceMode. Supported values are 'token' and 'ssm'. If token is specified, and the GITHUB_TOKEN environment variable is not set, the 'ssm' serviceMode is used as an automatic fallback.
      --github.ssm-key string            [GITHUB_APP_SSM_ARN] The SSM parameter key to use when fetching GitHub App credentials
      --github.webhook-secret string     [GITHUB_WEBHOOK_SECRET] The secret to use when validating incoming GitHub webhook payloads. If not specified, no validation is performed
  -h, --help                             help for this command
  -V, --logger.caller-trace              [CALLER_TRACE] Enable caller trace in logs
  -v, --logger.verbose count             [VERBOSITY] Increase logger verbosity (default WarnLevel)
      --promotion.dynamic                [PROMOTION_DYNAMIC] Enable dynamic promotion
      --promotion.dynamic-key string     [PROMOTION_DYNAMIC_KEY] The key to use when fetching the dynamic promoter configuration (default "gitops-promotion-path")
      --service                          [SERVICE_MODE] If set to true, the service will run in 'service' mode. Otherwise, it will run in 'lambda' mode by default
```

## Contributing

* Feel free to open an issue describing the problem you are facing or the feature you want to see implemented.
* If you want to contribute, fork the repository and submit a pull request.
    * Make sure to follow the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) specification.
