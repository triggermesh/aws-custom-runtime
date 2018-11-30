### AWS custom runtime

Proof of concept, WIP

Knative buildtemplate to run AWS Lambda custom runtime functions 

This repository contains `example` lambda function with AWS custom runtime described in [tutorial](https://docs.aws.amazon.com/lambda/latest/dg/runtimes-walkthrough.html). To run this function run:

1. Install AWS custom runtime buildtemplate:
```
tm deploy buildtemplate -f https://raw.githubusercontent.com/triggermesh/aws-custom-runtime/master/buildtemplate.yaml
```

2. Deploy function:
```
tm deploy service lambda-foo -f https://github.com/triggermesh/aws-custom-runtime --build-template aws-custom-runtime --build-argument DIRECTORY=example --wait
```

In output you'll see URL that you can use to access `example/function.sh` function