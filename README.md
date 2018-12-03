## Running AWS Lambda Custom Runtime in Knative

In November 2018, AWS announced support for [Lambda custom runtime](https://aws.amazon.com/about-aws/whats-new/2018/11/aws-lambda-now-supports-custom-runtimes-and-layers/) using a straightforward [AWS lambda runtime API](https://docs.aws.amazon.com/lambda/latest/dg/runtimes-api.html).

In this repository you find a function invoker implemented in Go, which provides the AWS Lambda runtime API. You also find a Knative build template. Using this build template you can run AWS Lambda custom runtimes directly in your Kubernetes cluster using [Knative](https://github.com/knative).

The AWS Lambdas execution [environment](https://docs.aws.amazon.com/lambda/latest/dg/current-supported-versions.html) is replicated using the Docker image `amazonlinux` and some environment variables.

### AWS custom runtime walkthrough

This repository contains an `example` lambda function written in bash with a AWS custom runtime described in [tutorial](https://docs.aws.amazon.com/lambda/latest/dg/runtimes-walkthrough.html). To run this function:

1. Install AWS custom runtime buildtemplate:
```
tm deploy buildtemplate -f https://raw.githubusercontent.com/triggermesh/aws-custom-runtime/master/buildtemplate.yaml
```

2. Deploy function:
```
tm deploy service lambda-bash -f https://github.com/triggermesh/aws-custom-runtime --build-template aws-custom-runtime --build-argument DIRECTORY=example --wait
```

In output you'll see URL that you can use to access `example/function.sh` function


### AWS Lambda RUST example

RUST custom runtime is also verified to be compatible with this buildtemplate. Though [official readme](https://github.com/awslabs/aws-lambda-rust-runtime) has build instructions, it is more convenient to use docker.

1. Clone repository:
```
git clone https://github.com/awslabs/aws-lambda-rust-runtime
cd aws-lambda-rust-runtime
```

2. Build binary and rename it to `bootstrap`:
```
docker run --rm --user "$(id -u)":"$(id -g)" -v "$PWD":/usr/src/myapp -w /usr/src/myapp rust:1.30.0 cargo build -p lambda_runtime --example basic --release
mv target/release/examples/basic target/release/examples/bootstrap
```

3. Deploy buildtemplate function using [`tm`](https://github.com/triggermesh/tm) CLI.
```
tm deploy buildtemplate -f https://raw.githubusercontent.com/triggermesh/aws-custom-runtime/master/buildtemplate.yaml
tm deploy service lambda-rust -f target/release/examples/ --build-template aws-custom-runtime
```

Use your RUST AWS Lambda function on knative:

```
curl lambda-rust.default.k.triggermesh.io --data '{"firstName": "Foo"}'
{"message":"Hello, Foo!"}
```

### Support

We would love your feedback on this tool so don't hesitate to let us know what is wrong and how we could improve it, just file an [issue](https://github.com/triggermesh/aws-custom-runtime/issues/new)

### Code of Conduct

This plugin is by no means part of [CNCF](https://www.cncf.io/) but we abide by its [code of conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md)
