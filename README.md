# Lambda Local Runner

This program allows you to run your lambda functions behind a REST interface (like a local copy of API Gateway). It provides similar functionality to [AWS SAM][aws-sam] without specifically being tied to AWS or the serverless model. This means that the lambda functions do not have to be defined via SAM. Also SAM builds and deploys your lambda functions, whereas this program is designed just to run locally.

It gets around some problems I have with SAM with the local workflow. Currently SAM requires running the `build` command every time you change your code, which in turn re-packages the whole lambda function including dependencies. This can be slow with lots of dependencies. This program is designed to run without rebuilding your package.

It assumes that when

## Features

- [ ] read real cloudformation templates (sam or base)
- [ ] host a real web API
- [ ] run your code in docker

## Links

- https://hub.docker.com/r/lambci/lambda

[aws-sam]: #TODO
