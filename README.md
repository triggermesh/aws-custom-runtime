[![Go Report Card](https://goreportcard.com/badge/github.com/triggermesh/aws-custom-runtime)](https://goreportcard.com/report/github.com/triggermesh/aws-custom-runtime) [![CircleCI](https://circleci.com/gh/triggermesh/aws-custom-runtime.svg?style=shield)](https://circleci.com/gh/triggermesh/aws-custom-runtime)

## Running AWS Lambda Custom Runtime in Knative

In November 2018, AWS announced support for [Lambda custom runtime](https://aws.amazon.com/about-aws/whats-new/2018/11/aws-lambda-now-supports-custom-runtimes-and-layers/) using a straightforward [AWS lambda runtime API](https://docs.aws.amazon.com/lambda/latest/dg/runtimes-api.html).

In this repository you find a function invoker implemented in Go, which provides the AWS Lambda runtime API. You also find a Knative build template. Using this build template you can run AWS Lambda custom runtimes directly in your Kubernetes cluster using [Knative](https://github.com/knative).

The AWS Lambdas execution [environment](https://docs.aws.amazon.com/lambda/latest/dg/current-supported-versions.html) is replicated using the Docker image `amazonlinux` and some environment variables.

### AWS custom runtime walkthrough

This repository contains an `example` lambda function written in bash with a AWS custom runtime described in this AWS [tutorial](https://docs.aws.amazon.com/lambda/latest/dg/runtimes-walkthrough.html). To run this function use our [`tm`](https://github.com/triggermesh/tm) client to talk to the knative API.

1. Install AWS custom runtime:
```
tm deploy task -f https://raw.githubusercontent.com/triggermesh/aws-custom-runtime/master/runtime.yaml
```

2. Deploy function:
```
tm deploy service lambda-bash -f https://github.com/triggermesh/aws-custom-runtime --runtime aws-custom-runtime --build-argument DIRECTORY=example --wait
```

In output you'll see URL that you can use to access `example/function.sh` function


### AWS Lambda RUST example

RUST is also verified to be compatible with this runtime. Though [official readme](https://github.com/awslabs/aws-lambda-rust-runtime) has build instructions, it is more convenient to use docker.

1. Clone repository:
```
git clone https://github.com/awslabs/aws-lambda-rust-runtime
cd aws-lambda-rust-runtime
```

2. Build binary and rename it to `bootstrap`:
```
docker run --rm --user "$(id -u)":"$(id -g)" -v "$PWD":/usr/src/myapp -w /usr/src/myapp rust:1.31.0 cargo build -p lambda_runtime --example basic --release
mv target/release/examples/basic target/release/examples/bootstrap
```

3. Deploy runtime using [`tm`](https://github.com/triggermesh/tm) CLI:
```
tm deploy runtime -f https://raw.githubusercontent.com/triggermesh/aws-custom-runtime/master/runtime.yaml
tm deploy service lambda-rust -f target/release/examples/ --runtime aws-custom-runtime
```

Use your RUST AWS Lambda function on knative:

```
curl lambda-rust.default.k.triggermesh.io --data '{"firstName": "Foo"}'
{"message":"Hello, Foo!"}
```

### AWS Lambda C++ example

1. Build custom runtime:
```
cd /tmp
git clone https://github.com/awslabs/aws-lambda-cpp.git
cd aws-lambda-cpp
mkdir build
cd build
cmake .. -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DCMAKE_INSTALL_PREFIX=/tmp/out
make && make install
```

2. Prepare example function:
```
mkdir /tmp/hello-cpp-world
cd /tmp/hello-cpp-world


cat > main.cpp <<EOF
// main.cpp
#include <aws/lambda-runtime/runtime.h>

using namespace aws::lambda_runtime;

invocation_response my_handler(invocation_request const& request)
{
   return invocation_response::success("Hello, World!", "application/json");
}

int main()
{
   run_handler(my_handler);
   return 0;
}
EOF


cat > CMakeLists.txt <<EOF
cmake_minimum_required(VERSION 3.5)
set(CMAKE_CXX_STANDARD 11)
project(bootstrap LANGUAGES CXX)

find_package(aws-lambda-runtime REQUIRED)
add_executable(\${PROJECT_NAME} "main.cpp")
target_link_libraries(\${PROJECT_NAME} PUBLIC AWS::aws-lambda-runtime)
aws_lambda_package_target(\${PROJECT_NAME})
EOF
```

3. Build function:
```
mkdir build
cd build
cmake .. -DCMAKE_BUILD_TYPE=Release -DCMAKE_PREFIX_PATH=/tmp/out
make
```

4. Deploy with [`tm`](https://github.com/triggermesh/tm) CLI:
```
tm deploy task -f https://raw.githubusercontent.com/triggermesh/aws-custom-runtime/master/runtime.yaml
tm deploy service lambda-cpp -f . --runtime aws-custom-runtime
```

C++ Lambda function is running on knative platform:
```
curl lambda-cpp.default.k.triggermesh.io --data '{"payload": "foobar"}'
Hello, World!
```


### Support

We would love your feedback on this tool so don't hesitate to let us know what is wrong and how we could improve it, just file an [issue](https://github.com/triggermesh/aws-custom-runtime/issues/new)

### Code of Conduct

This plugin is by no means part of [CNCF](https://www.cncf.io/) but we abide by its [code of conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md)
