# Lambda Local Runner

![CI status](https://github.com/mindriot101/lambda-local-runner/actions/workflows/go.yml/badge.svg?branch=main)

- [Features](#features)
- [Installation](#installation)
- [Usage](#usage)

This program allows you to run your lambda functions behind a REST interface (like a local copy of API Gateway). It provides similar functionality to [AWS SAM][aws-sam] without specifically being tied to AWS or the serverless model. This means that the lambda functions do not have to be defined via SAM. Also SAM builds and deploys your lambda functions, whereas this program is designed just to run locally.

It gets around some problems I have with SAM with the local workflow. Currently SAM requires running the `build` command every time you change your code, which in turn re-packages the whole lambda function including dependencies. This can be slow with lots of dependencies. This program is designed to run without rebuilding your package.

It assumes that when your code changes, you haven't changed your dependencies. This leads to a much faster round-trip time from your code changing to your test request working. Ideally the time taken to reload the lambda function should be less than you running your HTTP request.

## Features

- Read real cloudformation templates (SAM or base)
- Host a real web API which accepts requests
- Rapid turnaround time from your code changing to the new code being available from the web server

## Installation

### Prerequisites

- `docker`

### Installation process

Download the latest release for your platform from the [releases] page, otherwise if you have a working Go compiler:

```
# install a specific release
go install github.com/mindriot101/lambda-local-runner@<tag>

# install from main branch
go install github.com/mindriot101/lambda-local-runner@latest

# fetch the source code and install
git clone https://github.com/mindriot101/lambda-local-runner
cd lambda-local-runner
go install
```

## Usage

The program needs to know the directory your lambda is unpacked to. This is specified The easiest way to set this up is with AWS SAM. After creating a project with SAM, run `sam build --use-container`. This creates the `.aws-sam/build` directory. Inside this is one directory per logical lambda resource defined in the `template.yaml`. You can then either edit the code in situ, or move this unpacked directory somewhere, and edit the files within. `lambda-local-runner` will pick up changes to any file in this directory, and restart the hosted lambdas. _Note: any compiled dependencies must be built for the correct architecture, hence the `--use-container` flag for `sam build`._

In addition, `lambda-local-runner` needs to know the CloudFormation template that specifies your lambdas. For a sam project, this is `template.yaml`.

To use the command, run:

```
# assuming a SAM project at <project_dir>, created with "sam init --name my_lambda"
lambda-local-runner -r <project_dir>/.aws-sam/build <project_dir>/template.yaml
```

This spawns a contianer per lambda event mapping (i.e. each endpoint defined), and a web server that listens on port 8080. Requests can be sent to this web server using the endpoints defined in your CloudFormation template.

### Example

Let's say you have a project `my_lambda`, with the template:

```yaml
Resources:
  Function:
    Type: AWS::Serverless::Function
    # other properties
    Handler: app.lambda_handler.py
    CodeUri: hello_world/
    Events:
      Hello:
        Type: Api
        Properties:
          Path: /hello
          Method: get
```

at `my_lambda/template.yaml`, and `my_lambda/hello_world/app.py` contains the following:

```python
import json

def lambda_handler(event, context):
    return {
        "statusCode": 200,
        "body": json.dumps({"message": "Hello world"}),
    }
```

Building the project with `sam build --use-container` creates a `.aws-sam/build/Function` directory with your "packaged" lambda function code.

Run `lambda-local-runner` as the following:

```
lambda-local-runner -r my_lambda/.aws-sam/build my_lambda/template.yaml
```

and make your request:

```
curl http://localhost:8080/hello
# => {"message": "Hello world"}
```

[aws-sam]: https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/serverless-sam-cli-install.html
[releases]: https://github.com/mindriot101/lambda-local-runner/releases
