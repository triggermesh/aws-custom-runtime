### AWS custom runtime

Proof of concept, WIP

Knative buildtemplate to run AWS Lambda custom runtime functions 

This repository contains `example` lambda function with AWS custom runtime described in [tutorial](https://docs.aws.amazon.com/lambda/latest/dg/runtimes-walkthrough.html). To run this function:

1. Install AWS custom runtime buildtemplate:
```
tm deploy buildtemplate -f https://raw.githubusercontent.com/triggermesh/aws-custom-runtime/master/buildtemplate.yaml
```

2. Deploy function:
```
tm deploy service lambda-bash -f https://github.com/triggermesh/aws-custom-runtime --build-template aws-custom-runtime --build-argument DIRECTORY=example --wait
```

In output you'll see URL that you can use to access `example/function.sh` function


### AWS Lambda Rust example

Rust custom runtime is also verified to be compatible with this buildtemplate. Though [official readme](https://github.com/awslabs/aws-lambda-rust-runtime) has build instructions, I found it more convenient to use docker.

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

3. Deploy buildtemplate function using [tm CLI](https://github.com/triggermesh/tm)
```
tm deploy buildtemplate -f https://raw.githubusercontent.com/triggermesh/aws-custom-runtime/master/buildtemplate.yaml
tm deploy service lambda-rust -f target/release/examples/ --build-template aws-custom-runtime
```

Use your Rust AWS Lambda function on knative:

![image](https://user-images.githubusercontent.com/13515865/49390178-66455980-f752-11e8-83c1-ac6f463012aa.png)
